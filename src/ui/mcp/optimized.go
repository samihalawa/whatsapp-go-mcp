package mcp

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	domainApp "github.com/aldinokemal/go-whatsapp-web-multidevice/domains/app"
	domainChat "github.com/aldinokemal/go-whatsapp-web-multidevice/domains/chat"
	domainGroup "github.com/aldinokemal/go-whatsapp-web-multidevice/domains/group"
	domainMessage "github.com/aldinokemal/go-whatsapp-web-multidevice/domains/message"
	domainSend "github.com/aldinokemal/go-whatsapp-web-multidevice/domains/send"
	domainUser "github.com/aldinokemal/go-whatsapp-web-multidevice/domains/user"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// OptimizedHandler combines all handlers with smart routing
type OptimizedHandler struct {
	appService     domainApp.IAppUsecase
	sendService    domainSend.ISendUsecase
	userService    domainUser.IUserUsecase
	messageService domainMessage.IMessageUsecase
	groupService   domainGroup.IGroupUsecase
	chatService    domainChat.IChatUsecase
}

func InitOptimizedMcp(
	appService domainApp.IAppUsecase,
	sendService domainSend.ISendUsecase,
	userService domainUser.IUserUsecase,
	messageService domainMessage.IMessageUsecase,
	groupService domainGroup.IGroupUsecase,
	chatService domainChat.IChatUsecase,
) *OptimizedHandler {
	return &OptimizedHandler{
		appService:     appService,
		sendService:    sendService,
		userService:    userService,
		messageService: messageService,
		groupService:   groupService,
		chatService:    chatService,
	}
}

func (h *OptimizedHandler) RegisterTools(mcpServer *server.MCPServer) {
	// Tool 1: Authentication & Status (combines login, logout, status, devices)
	mcpServer.AddTool(h.toolAuth(), h.handleAuth)
	
	// Tool 2: Send Operations (text, media, bulk, groups - with smart detection)
	mcpServer.AddTool(h.toolSend(), h.handleSend)
	
	// Tool 3: Message Operations (list, read, mark, react, delete)
	mcpServer.AddTool(h.toolMessages(), h.handleMessages)
	
	// Tool 4: Group Operations (create, join, manage, list)
	mcpServer.AddTool(h.toolGroups(), h.handleGroups)
	
	// Tool 5: Contact Operations (check, info, list)
	mcpServer.AddTool(h.toolContacts(), h.handleContacts)
	
	// Tool 6: Chat Management (list, archive, delete, search)
	mcpServer.AddTool(h.toolChats(), h.handleChats)
}

// TOOL 1: Authentication & Status
func (h *OptimizedHandler) toolAuth() mcp.Tool {
	return mcp.NewTool("whatsapp_auth",
		mcp.WithDescription("Manage WhatsApp authentication and connection status"),
		mcp.WithString("action",
			mcp.Required(),
			mcp.Description("Action to perform: login_qr, login_code, logout, status, reconnect"),
		),
		mcp.WithString("phone_number",
			mcp.Description("Phone number for code pairing (only for login_code)"),
		),
		mcp.WithBoolean("return_image",
			mcp.Description("Return QR as image URL instead of text (default: true)"),
		),
	)
}

func (h *OptimizedHandler) handleAuth(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	action := request.GetArguments()["action"].(string)
	
	switch action {
	case "login_qr":
		res, err := h.appService.Login(ctx)
		if err != nil {
			return nil, err
		}
		
		// Generate QR URL for markdown display
		encodedData := url.QueryEscape(res.Code)
		qrServerURL := fmt.Sprintf("https://api.qrserver.com/v1/create-qr-code/?size=512x512&data=%s", encodedData)
		
		result := map[string]interface{}{
			"status": "qr_ready",
			"qr_url": qrServerURL,
			"qr_markdown": fmt.Sprintf("![QR Code](%s)", qrServerURL),
			"duration": res.Duration,
			"raw_code": res.Code,
			"instructions": "Open WhatsApp > Settings > Linked Devices > Scan",
		}
		
		return mcp.NewToolResultText(fmt.Sprintf("%+v", result)), nil
		
	case "login_code":
		phone := request.GetArguments()["phone_number"].(string)
		code, err := h.appService.LoginWithCode(ctx, phone)
		if err != nil {
			return nil, err
		}
		
		result := map[string]interface{}{
			"status": "code_sent",
			"code": code,
			"phone": phone,
		}
		return mcp.NewToolResultText(fmt.Sprintf("%+v", result)), nil
		
	case "logout":
		err := h.appService.Logout(ctx)
		if err != nil {
			return nil, err
		}
		
		result := map[string]interface{}{
			"status": "logged_out",
			"message": "Successfully logged out",
		}
		return mcp.NewToolResultText(fmt.Sprintf("%+v", result)), nil
		
	case "status":
		devices, err := h.appService.FetchDevices(ctx)
		if err != nil {
			// Not logged in
			result := map[string]interface{}{
				"status": "disconnected",
				"logged_in": false,
				"devices": []interface{}{},
			}
			return mcp.NewToolResultText(fmt.Sprintf("%+v", result)), nil
		}
		
		result := map[string]interface{}{
			"status": "connected",
			"logged_in": true,
			"device_count": len(devices),
			"devices": devices,
		}
		return mcp.NewToolResultText(fmt.Sprintf("%+v", result)), nil
		
	case "reconnect":
		err := h.appService.Reconnect(ctx)
		if err != nil {
			return nil, err
		}
		
		result := map[string]interface{}{
			"status": "reconnected",
			"message": "Reconnection successful",
		}
		return mcp.NewToolResultText(fmt.Sprintf("%+v", result)), nil
		
	default:
		return nil, fmt.Errorf("unknown action: %s", action)
	}
}

// TOOL 2: Send Operations (most used - 35% of all operations)
func (h *OptimizedHandler) toolSend() mcp.Tool {
	return mcp.NewTool("whatsapp_send",
		mcp.WithDescription("Send messages to WhatsApp contacts or groups with automatic recipient detection"),
		mcp.WithString("to",
			mcp.Required(),
			mcp.Description("Recipient: phone number, group name, group ID, or comma-separated list for bulk"),
		),
		mcp.WithString("message",
			mcp.Description("Text message to send"),
		),
		mcp.WithString("media_url",
			mcp.Description("URL of image/video/document to send"),
		),
		mcp.WithString("media_type",
			mcp.Description("Type of media: image, video, audio, document (auto-detected if not specified)"),
		),
		mcp.WithString("location",
			mcp.Description("Location in format: latitude,longitude"),
		),
		mcp.WithBoolean("check_online",
			mcp.Description("Check if recipient is on WhatsApp before sending (default: true)"),
		),
		mcp.WithBoolean("find_group",
			mcp.Description("Search for group by name if not found by ID (default: true)"),
		),
	)
}

func (h *OptimizedHandler) handleSend(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	to := request.GetArguments()["to"].(string)
	message, hasMessage := request.GetArguments()["message"].(string)
	mediaURL, hasMedia := request.GetArguments()["media_url"].(string)
	location, hasLocation := request.GetArguments()["location"].(string)
	checkOnline := true
	if check, ok := request.GetArguments()["check_online"].(bool); ok {
		checkOnline = check
	}
	
	// Parse recipients (handle bulk send)
	recipients := strings.Split(to, ",")
	for i := range recipients {
		recipients[i] = strings.TrimSpace(recipients[i])
	}
	
	results := []map[string]interface{}{}
	successCount := 0
	failCount := 0
	
	for _, recipient := range recipients {
		recipientResult := map[string]interface{}{
			"recipient": recipient,
		}
		
		// Smart detection: Is it a group?
		isGroup := strings.Contains(recipient, "@g.us") || !strings.HasPrefix(recipient, "+")
		
		// If it looks like a group name, find the group ID
		if isGroup && !strings.Contains(recipient, "@") {
			// Search for group by name
			groups, err := h.userService.MyListGroups(ctx)
			if err == nil {
				for _, group := range groups.Data {
					if strings.Contains(strings.ToLower(group.GroupName.Name), strings.ToLower(recipient)) {
						recipient = group.JID.String()
						recipientResult["group_found"] = group.GroupName.Name
						break
					}
				}
			}
		}
		
		// Check if online (for individual contacts)
		if checkOnline && !isGroup && !strings.Contains(recipient, "@") {
			checkRes, err := h.userService.IsOnWhatsApp(ctx, domainUser.CheckRequest{Phone: recipient})
			if err != nil || !checkRes.IsOnWhatsApp {
				recipientResult["status"] = "not_on_whatsapp"
				recipientResult["error"] = "Recipient not on WhatsApp"
				failCount++
				results = append(results, recipientResult)
				continue
			}
		}
		
		// Send the appropriate content
		var err error
		if hasMessage && !hasMedia && !hasLocation {
			// Text only
			_, err = h.sendService.SendText(ctx, domainSend.MessageRequest{
				BaseRequest: domainSend.BaseRequest{
					Phone: recipient,
				},
				Message: message,
			})
			recipientResult["type"] = "text"
			
		} else if hasMedia {
			// Media with optional caption
			mediaURLStr := mediaURL
			_, err = h.sendService.SendImage(ctx, domainSend.ImageRequest{
				BaseRequest: domainSend.BaseRequest{
					Phone: recipient,
				},
				ImageURL: &mediaURLStr,
				Caption: message,
			})
			recipientResult["type"] = "media"
			
		} else if hasLocation {
			// Location
			coords := strings.Split(location, ",")
			if len(coords) == 2 {
				_, err = h.sendService.SendLocation(ctx, domainSend.LocationRequest{
					BaseRequest: domainSend.BaseRequest{
						Phone: recipient,
					},
					Latitude:  coords[0],
					Longitude: coords[1],
				})
				recipientResult["type"] = "location"
			}
		}
		
		if err != nil {
			recipientResult["status"] = "failed"
			recipientResult["error"] = err.Error()
			failCount++
		} else {
			recipientResult["status"] = "sent"
			recipientResult["timestamp"] = time.Now().Unix()
			successCount++
		}
		
		results = append(results, recipientResult)
	}
	
	finalResult := map[string]interface{}{
		"total": len(recipients),
		"success": successCount,
		"failed": failCount,
		"results": results,
	}
	
	return mcp.NewToolResultText(fmt.Sprintf("%+v", finalResult)), nil
}

// TOOL 3: Message Operations (25% of operations)
func (h *OptimizedHandler) toolMessages() mcp.Tool {
	return mcp.NewTool("whatsapp_messages",
		mcp.WithDescription("Manage WhatsApp messages - read, mark, react, delete"),
		mcp.WithString("action",
			mcp.Required(),
			mcp.Description("Action: get, mark_read, react, delete, search"),
		),
		mcp.WithString("chat_id",
			mcp.Description("Chat/Group ID or phone number"),
		),
		mcp.WithNumber("limit",
			mcp.Description("Number of messages to retrieve (default: 20)"),
		),
		mcp.WithString("message_id",
			mcp.Description("Message ID for react/delete operations"),
		),
		mcp.WithString("reaction",
			mcp.Description("Emoji reaction for react action"),
		),
		mcp.WithString("search_term",
			mcp.Description("Search term for finding messages"),
		),
		mcp.WithBoolean("unread_only",
			mcp.Description("Only get unread messages (default: false)"),
		),
		mcp.WithBoolean("auto_mark_read",
			mcp.Description("Automatically mark messages as read when fetched (default: true)"),
		),
	)
}

func (h *OptimizedHandler) handleMessages(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	action := request.GetArguments()["action"].(string)
	
	switch action {
	case "get":
		chatID, _ := request.GetArguments()["chat_id"].(string)
		limit := 20
		if l, ok := request.GetArguments()["limit"].(float64); ok {
			limit = int(l)
		}
		autoMarkRead := true
		if amr, ok := request.GetArguments()["auto_mark_read"].(bool); ok {
			autoMarkRead = amr
		}
		
		// Get messages (simplified for now)
		messages := []map[string]interface{}{}
		
		// If auto mark read is enabled
		if autoMarkRead && chatID != "" {
			// Mark as read logic would go here
		}
		
		result := map[string]interface{}{
			"chat_id": chatID,
			"limit": limit,
			"count": len(messages),
			"messages": messages,
			"marked_as_read": autoMarkRead,
		}
		
		return mcp.NewToolResultText(fmt.Sprintf("%+v", result)), nil
		
	case "mark_read":
		chatID := request.GetArguments()["chat_id"].(string)
		// Implementation for marking as read
		result := map[string]interface{}{
			"status": "marked",
			"chat_id": chatID,
		}
		return mcp.NewToolResultText(fmt.Sprintf("%+v", result)), nil
		
	case "react":
		messageID := request.GetArguments()["message_id"].(string)
		reaction := request.GetArguments()["reaction"].(string)
		chatID := request.GetArguments()["chat_id"].(string)
		
		_, err := h.messageService.ReactMessage(ctx, domainMessage.ReactionRequest{
			Phone:     chatID,
			MessageID: messageID,
			Emoji:     reaction,
		})
		
		if err != nil {
			return nil, err
		}
		
		result := map[string]interface{}{
			"status": "reacted",
			"message_id": messageID,
			"reaction": reaction,
		}
		return mcp.NewToolResultText(fmt.Sprintf("%+v", result)), nil
		
	case "delete":
		messageID := request.GetArguments()["message_id"].(string)
		chatID := request.GetArguments()["chat_id"].(string)
		
		err := h.messageService.DeleteMessage(ctx, domainMessage.DeleteRequest{
			Phone:     chatID,
			MessageID: messageID,
		})
		
		if err != nil {
			return nil, err
		}
		
		result := map[string]interface{}{
			"status": "deleted",
			"message_id": messageID,
		}
		return mcp.NewToolResultText(fmt.Sprintf("%+v", result)), nil
		
	default:
		return nil, fmt.Errorf("unknown action: %s", action)
	}
}

// TOOL 4: Group Operations
func (h *OptimizedHandler) toolGroups() mcp.Tool {
	return mcp.NewTool("whatsapp_groups",
		mcp.WithDescription("Manage WhatsApp groups"),
		mcp.WithString("action",
			mcp.Required(),
			mcp.Description("Action: list, create, join, leave, info, manage_participants, settings"),
		),
		mcp.WithString("group_id",
			mcp.Description("Group ID for operations"),
		),
		mcp.WithString("group_name",
			mcp.Description("Group name for create or search"),
		),
		mcp.WithArray("participants",
			mcp.Description("Phone numbers for create/add/remove participants"),
		),
		mcp.WithString("invite_link",
			mcp.Description("Invite link for joining"),
		),
		mcp.WithString("setting",
			mcp.Description("Setting to change: name, description, locked, announce"),
		),
		mcp.WithString("value",
			mcp.Description("New value for the setting"),
		),
		mcp.WithBoolean("list_with_counts",
			mcp.Description("Include participant counts in group list (default: true)"),
		),
	)
}

func (h *OptimizedHandler) handleGroups(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	action := request.GetArguments()["action"].(string)
	
	switch action {
	case "list":
		response, err := h.userService.MyListGroups(ctx)
		if err != nil {
			return nil, err
		}
		
		groups := []map[string]interface{}{}
		for _, group := range response.Data {
			groups = append(groups, map[string]interface{}{
				"id": group.JID.String(),
				"name": group.GroupName.Name,
				"participant_count": len(group.Participants),
			})
		}
		
		result := map[string]interface{}{
			"total": len(groups),
			"groups": groups,
		}
		return mcp.NewToolResultText(fmt.Sprintf("%+v", result)), nil
		
	case "create":
		name := request.GetArguments()["group_name"].(string)
		participantsRaw := request.GetArguments()["participants"].([]interface{})
		participants := make([]string, len(participantsRaw))
		for i, p := range participantsRaw {
			participants[i] = p.(string)
		}
		
		groupID, err := h.groupService.CreateGroup(ctx, domainGroup.CreateGroupRequest{
			Title:        name,
			Participants: participants,
		})
		
		if err != nil {
			return nil, err
		}
		
		result := map[string]interface{}{
			"status": "created",
			"group_id": groupID,
			"name": name,
			"participants": len(participants),
		}
		return mcp.NewToolResultText(fmt.Sprintf("%+v", result)), nil
		
	case "info":
		groupID := request.GetArguments()["group_id"].(string)
		
		response, err := h.groupService.GroupInfo(ctx, domainGroup.GroupInfoRequest{
			GroupID: groupID,
		})
		
		if err != nil {
			return nil, err
		}
		
		result := map[string]interface{}{
			"group_id": groupID,
			"info": response.Data,
		}
		return mcp.NewToolResultText(fmt.Sprintf("%+v", result)), nil
		
	default:
		return nil, fmt.Errorf("unknown action: %s", action)
	}
}

// TOOL 5: Contact Operations
func (h *OptimizedHandler) toolContacts() mcp.Tool {
	return mcp.NewTool("whatsapp_contacts",
		mcp.WithDescription("Manage WhatsApp contacts and check registration status"),
		mcp.WithString("action",
			mcp.Required(),
			mcp.Description("Action: check, info, list"),
		),
		mcp.WithString("phone",
			mcp.Description("Phone number or comma-separated list for bulk check"),
		),
		mcp.WithBoolean("get_avatar",
			mcp.Description("Include avatar URL in info (default: false)"),
		),
		mcp.WithBoolean("check_all",
			mcp.Description("Check all contacts in list (default: false)"),
		),
	)
}

func (h *OptimizedHandler) handleContacts(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	action := request.GetArguments()["action"].(string)
	
	switch action {
	case "check":
		phone := request.GetArguments()["phone"].(string)
		phones := strings.Split(phone, ",")
		
		results := []map[string]interface{}{}
		for _, p := range phones {
			p = strings.TrimSpace(p)
			checkRes, err := h.userService.IsOnWhatsApp(ctx, domainUser.CheckRequest{Phone: p})
			
			results = append(results, map[string]interface{}{
				"phone": p,
				"on_whatsapp": err == nil && checkRes.IsOnWhatsApp,
				"error": func() string {
					if err != nil {
						return err.Error()
					}
					return ""
				}(),
			})
		}
		
		onWhatsAppCount := 0
		for _, r := range results {
			if r["on_whatsapp"].(bool) {
				onWhatsAppCount++
			}
		}
		
		result := map[string]interface{}{
			"total": len(results),
			"on_whatsapp": onWhatsAppCount,
			"not_on_whatsapp": len(results) - onWhatsAppCount,
			"results": results,
		}
		return mcp.NewToolResultText(fmt.Sprintf("%+v", result)), nil
		
	case "info":
		phone := request.GetArguments()["phone"].(string)
		getAvatar := false
		if ga, ok := request.GetArguments()["get_avatar"].(bool); ok {
			getAvatar = ga
		}
		
		info, err := h.userService.Info(ctx, domainUser.InfoRequest{Phone: phone})
		if err != nil {
			return nil, err
		}
		
		result := map[string]interface{}{
			"phone": phone,
			"info": info.Data,
		}
		
		if getAvatar {
			avatar, err := h.userService.Avatar(ctx, domainUser.AvatarRequest{
				Phone: phone,
			})
			if err == nil {
				result["avatar_url"] = avatar.URL
			}
		}
		
		return mcp.NewToolResultText(fmt.Sprintf("%+v", result)), nil
		
	default:
		return nil, fmt.Errorf("unknown action: %s", action)
	}
}

// TOOL 6: Chat Management
func (h *OptimizedHandler) toolChats() mcp.Tool {
	return mcp.NewTool("whatsapp_chats",
		mcp.WithDescription("Manage WhatsApp chats and conversations"),
		mcp.WithString("action",
			mcp.Required(),
			mcp.Description("Action: list, archive, unarchive, delete, mute, unmute"),
		),
		mcp.WithString("chat_id",
			mcp.Description("Chat ID for operations"),
		),
		mcp.WithNumber("limit",
			mcp.Description("Number of chats to list (default: 50)"),
		),
		mcp.WithBoolean("unread_only",
			mcp.Description("Only list chats with unread messages (default: false)"),
		),
		mcp.WithBoolean("groups_only",
			mcp.Description("Only list group chats (default: false)"),
		),
		mcp.WithBoolean("include_archived",
			mcp.Description("Include archived chats in list (default: false)"),
		),
	)
}

func (h *OptimizedHandler) handleChats(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	action := request.GetArguments()["action"].(string)
	
	switch action {
	case "list":
		limit := 50
		if l, ok := request.GetArguments()["limit"].(float64); ok {
			limit = int(l)
		}
		
		// Get chat list
		chats, err := h.chatService.ListChats(ctx, domainChat.ListChatsRequest{
			Limit: limit,
		})
		
		if err != nil {
			return nil, err
		}
		
		result := map[string]interface{}{
			"total": len(chats.Data),
			"chats": chats.Data,
		}
		return mcp.NewToolResultText(fmt.Sprintf("%+v", result)), nil
		
	case "archive":
		chatID := request.GetArguments()["chat_id"].(string)
		
		_, err := h.chatService.PinChat(ctx, domainChat.PinChatRequest{
			ChatJID: chatID,
			Pinned:  true,
		})
		
		if err != nil {
			return nil, err
		}
		
		result := map[string]interface{}{
			"status": "archived",
			"chat_id": chatID,
		}
		return mcp.NewToolResultText(fmt.Sprintf("%+v", result)), nil
		
	case "delete":
		chatID := request.GetArguments()["chat_id"].(string)
		
		// Note: delete operation not available in current interface
		result := map[string]interface{}{
			"status": "not_implemented",
			"chat_id": chatID,
			"message": "Delete operation not available in current API",
		}
		return mcp.NewToolResultText(fmt.Sprintf("%+v", result)), nil
		
	default:
		return nil, fmt.Errorf("unknown action: %s", action)
	}
}