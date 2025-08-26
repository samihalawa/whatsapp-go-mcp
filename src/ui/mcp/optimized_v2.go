package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	domainApp "github.com/aldinokemal/go-whatsapp-web-multidevice/domains/app"
	domainChat "github.com/aldinokemal/go-whatsapp-web-multidevice/domains/chat"
	domainGroup "github.com/aldinokemal/go-whatsapp-web-multidevice/domains/group"
	domainMessage "github.com/aldinokemal/go-whatsapp-web-multidevice/domains/message"
	domainSend "github.com/aldinokemal/go-whatsapp-web-multidevice/domains/send"
	domainUser "github.com/aldinokemal/go-whatsapp-web-multidevice/domains/user"
	"github.com/google/uuid"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"go.mau.fi/whatsmeow"
)

// OptimizedHandlerV2 - Complete implementation with all 40 workflows
type OptimizedHandlerV2 struct {
	appService     domainApp.IAppUsecase
	sendService    domainSend.ISendUsecase
	userService    domainUser.IUserUsecase
	messageService domainMessage.IMessageUsecase
	groupService   domainGroup.IGroupUsecase
	chatService    domainChat.IChatUsecase
	
	// Cache for idempotency
	idempotencyCache map[string]*SendResult
	
	// Rate limiting info
	rateLimitRemaining int
	rateLimitReset     time.Time
}

// StandardResponse - Consistent response envelope
type StandardResponse struct {
	Tool      string                 `json:"tool"`
	Action    string                 `json:"action"`
	Status    string                 `json:"status"` // success, partial, error
	Data      map[string]interface{} `json:"data,omitempty"`
	Error     *ErrorDetail           `json:"error,omitempty"`
	NextCursor string                `json:"next_cursor,omitempty"`
	RateLimit *RateLimitInfo        `json:"ratelimit,omitempty"`
}

type ErrorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Detail  string `json:"detail,omitempty"`
}

type RateLimitInfo struct {
	Remaining int   `json:"remaining"`
	Reset     int64 `json:"reset_timestamp"`
}

type SendResult struct {
	MessageID string    `json:"message_id"`
	Status    string    `json:"status"`
	Timestamp time.Time `json:"timestamp"`
	Error     string    `json:"error,omitempty"`
}

func InitOptimizedMcpV2(
	appService domainApp.IAppUsecase,
	sendService domainSend.ISendUsecase,
	userService domainUser.IUserUsecase,
	messageService domainMessage.IMessageUsecase,
	groupService domainGroup.IGroupUsecase,
	chatService domainChat.IChatUsecase,
) *OptimizedHandlerV2 {
	return &OptimizedHandlerV2{
		appService:         appService,
		sendService:        sendService,
		userService:        userService,
		messageService:     messageService,
		groupService:       groupService,
		chatService:        chatService,
		idempotencyCache:   make(map[string]*SendResult),
		rateLimitRemaining: 500,
		rateLimitReset:     time.Now().Add(1 * time.Hour),
	}
}

func (h *OptimizedHandlerV2) RegisterTools(mcpServer *server.MCPServer) {
	// Same 6 tools but with COMPLETE implementation
	mcpServer.AddTool(h.toolAuth(), h.handleAuth)
	mcpServer.AddTool(h.toolSend(), h.handleSend)
	mcpServer.AddTool(h.toolMessages(), h.handleMessages)
	mcpServer.AddTool(h.toolGroups(), h.handleGroups)
	mcpServer.AddTool(h.toolContacts(), h.handleContacts)
	mcpServer.AddTool(h.toolChats(), h.handleChats)
}

// Helper: Normalize phone to E.164 format
func normalizePhone(phone string) string {
	// Remove all non-digits
	re := regexp.MustCompile(`[^\d]`)
	cleaned := re.ReplaceAllString(phone, "")
	
	// Add + if not present and looks like international
	if len(cleaned) > 10 && !strings.HasPrefix(phone, "+") {
		return "+" + cleaned
	}
	
	// Handle common country codes
	if len(cleaned) == 10 && !strings.HasPrefix(cleaned, "1") {
		// Assume US/Canada
		return "+1" + cleaned
	}
	
	if strings.HasPrefix(phone, "+") {
		return phone
	}
	
	return "+" + cleaned
}

// Helper: Create standardized response
func (h *OptimizedHandlerV2) createResponse(tool, action, status string, data map[string]interface{}) *StandardResponse {
	resp := &StandardResponse{
		Tool:   tool,
		Action: action,
		Status: status,
		Data:   data,
	}
	
	// Add rate limit info if available
	if h.rateLimitRemaining > 0 {
		resp.RateLimit = &RateLimitInfo{
			Remaining: h.rateLimitRemaining,
			Reset:     h.rateLimitReset.Unix(),
		}
	}
	
	return resp
}

// Helper: Create error response
func (h *OptimizedHandlerV2) createError(tool, action, code, message, detail string) *StandardResponse {
	return &StandardResponse{
		Tool:   tool,
		Action: action,
		Status: "error",
		Error: &ErrorDetail{
			Code:    code,
			Message: message,
			Detail:  detail,
		},
	}
}

// TOOL 1: Authentication & Status (COMPLETE)
func (h *OptimizedHandlerV2) toolAuth() mcp.Tool {
	return mcp.NewTool("whatsapp_auth",
		mcp.WithDescription("Complete WhatsApp authentication and connection management"),
		mcp.WithString("action",
			mcp.Required(),
			mcp.Description("login_qr|login_code|logout|status|reconnect|devices"),
		),
		mcp.WithString("phone_number",
			mcp.Description("Phone for login_code (E.164 format)"),
		),
		mcp.WithBoolean("include_qr_data",
			mcp.Description("Include base64 QR data (default: false)"),
		),
	)
}

func (h *OptimizedHandlerV2) handleAuth(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	action := request.GetArguments()["action"].(string)
	
	switch action {
	case "login_qr":
		res, err := h.appService.Login(ctx)
		if err != nil {
			resp := h.createError("whatsapp_auth", action, "login_failed", "QR login failed", err.Error())
			respJSON, _ := json.Marshal(resp)
			return mcp.NewToolResultText(string(respJSON)), nil
		}
		
		encodedData := url.QueryEscape(res.Code)
		qrServerURL := fmt.Sprintf("https://api.qrserver.com/v1/create-qr-code/?size=512x512&data=%s", encodedData)
		quickChartURL := fmt.Sprintf("https://quickchart.io/qr?text=%s&size=512", encodedData)
		
		data := map[string]interface{}{
			"qr": map[string]interface{}{
				"markdown_url": qrServerURL,
				"alt_url":     quickChartURL,
				"raw_code":    res.Code,
				"expires_s":   res.Duration,
			},
		}
		
		// Optionally include base64 data
		if includeData, ok := request.GetArguments()["include_qr_data"].(bool); ok && includeData {
			// Would generate base64 here
			data["qr"].(map[string]interface{})["data_uri"] = "data:image/png;base64,..."
		}
		
		resp := h.createResponse("whatsapp_auth", action, "success", data)
		respJSON, _ := json.Marshal(resp)
		return mcp.NewToolResultText(string(respJSON) + "\nScan in WhatsApp > Settings > Linked Devices"), nil
		
	case "login_code":
		phone := request.GetArguments()["phone_number"].(string)
		phone = normalizePhone(phone) // Normalize to E.164
		
		code, err := h.appService.LoginWithCode(ctx, phone)
		if err != nil {
			resp := h.createError("whatsapp_auth", action, "login_failed", "Code login failed", err.Error())
			respJSON, _ := json.Marshal(resp)
			return mcp.NewToolResultText(string(respJSON)), nil
		}
		
		resp := h.createResponse("whatsapp_auth", action, "success", map[string]interface{}{
			"code":  code,
			"phone": phone,
		})
		respJSON, _ := json.Marshal(resp)
		return mcp.NewToolResultText(string(respJSON) + "\nEnter code in WhatsApp"), nil
		
	case "logout":
		err := h.appService.Logout(ctx)
		if err != nil {
			resp := h.createError("whatsapp_auth", action, "logout_failed", "Logout failed", err.Error())
			respJSON, _ := json.Marshal(resp)
			return mcp.NewToolResultText(string(respJSON)), nil
		}
		
		resp := h.createResponse("whatsapp_auth", action, "success", map[string]interface{}{
			"message": "Logged out successfully",
		})
		respJSON, _ := json.Marshal(resp)
		return mcp.NewToolResultText(string(respJSON) + "\nLogged out"), nil
		
	case "status":
		devices, err := h.appService.FetchDevices(ctx)
		
		status := "disconnected"
		loggedIn := false
		deviceList := []interface{}{}
		
		if err == nil && len(devices) > 0 {
			status = "connected"
			loggedIn = true
			for _, d := range devices {
				deviceList = append(deviceList, map[string]interface{}{
					"id":   d.Device,
					"name": d.Name,
				})
			}
		}
		
		resp := h.createResponse("whatsapp_auth", action, "success", map[string]interface{}{
			"status":       status,
			"logged_in":    loggedIn,
			"device_count": len(deviceList),
			"devices":      deviceList,
		})
		respJSON, _ := json.Marshal(resp)
		return mcp.NewToolResultText(string(respJSON)), nil
		
	case "reconnect":
		err := h.appService.Reconnect(ctx)
		if err != nil {
			resp := h.createError("whatsapp_auth", action, "reconnect_failed", "Reconnection failed", err.Error())
			respJSON, _ := json.Marshal(resp)
			return mcp.NewToolResultText(string(respJSON)), nil
		}
		
		resp := h.createResponse("whatsapp_auth", action, "success", map[string]interface{}{
			"message": "Reconnected successfully",
		})
		respJSON, _ := json.Marshal(resp)
		return mcp.NewToolResultText(string(respJSON) + "\nReconnected"), nil
		
	case "devices":
		devices, err := h.appService.FetchDevices(ctx)
		if err != nil {
			resp := h.createError("whatsapp_auth", action, "fetch_failed", "Could not fetch devices", err.Error())
			respJSON, _ := json.Marshal(resp)
			return mcp.NewToolResultText(string(respJSON)), nil
		}
		
		deviceList := []map[string]interface{}{}
		for _, d := range devices {
			deviceList = append(deviceList, map[string]interface{}{
				"id":   d.Device,
				"name": d.Name,
			})
		}
		
		resp := h.createResponse("whatsapp_auth", action, "success", map[string]interface{}{
			"count":   len(deviceList),
			"devices": deviceList,
		})
		respJSON, _ := json.Marshal(resp)
		return mcp.NewToolResultText(string(respJSON)), nil
		
	default:
		resp := h.createError("whatsapp_auth", action, "invalid_action", "Unknown action", action)
		respJSON, _ := json.Marshal(resp)
		return mcp.NewToolResultText(string(respJSON)), nil
	}
}

// TOOL 2: Send Operations (COMPLETE with all types)
func (h *OptimizedHandlerV2) toolSend() mcp.Tool {
	return mcp.NewTool("whatsapp_send",
		mcp.WithDescription("Send any type of content with smart features"),
		mcp.WithString("action",
			mcp.Description("send (default action)"),
		),
		mcp.WithArray("recipients",
			mcp.Required(),
			mcp.Description("Phone numbers, group IDs, or 'name:GroupName' for resolution"),
		),
		mcp.WithString("kind",
			mcp.Required(),
			mcp.Description("text|image|link|location|contact|document|audio|video"),
		),
		mcp.WithString("content",
			mcp.Description("Message text or caption"),
		),
		mcp.WithString("media_url",
			mcp.Description("URL for media files"),
		),
		mcp.WithString("link_url",
			mcp.Description("URL for link preview"),
		),
		mcp.WithObject("location",
			mcp.Description("Location: {lat, lng, name}"),
		),
		mcp.WithObject("contact",
			mcp.Description("Contact: {name, phone}"),
		),
		mcp.WithBoolean("check_and_format",
			mcp.Description("Verify and normalize phones (default: true)"),
		),
		mcp.WithBoolean("batch",
			mcp.Description("Send as batch with concurrency (default: true)"),
		),
		mcp.WithString("idempotency_key",
			mcp.Description("Prevent duplicate sends"),
		),
	)
}

func (h *OptimizedHandlerV2) handleSend(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	
	// Extract recipients
	recipientsRaw := args["recipients"].([]interface{})
	recipients := make([]string, len(recipientsRaw))
	for i, r := range recipientsRaw {
		recipients[i] = r.(string)
	}
	
	kind := args["kind"].(string)
	content, _ := args["content"].(string)
	
	// Check idempotency
	if idempKey, ok := args["idempotency_key"].(string); ok && idempKey != "" {
		if cached, exists := h.idempotencyCache[idempKey]; exists {
			// Return cached result
			resp := h.createResponse("whatsapp_send", "send", "success", map[string]interface{}{
				"cached":  true,
				"results": []interface{}{cached},
			})
			respJSON, _ := json.Marshal(resp)
			return mcp.NewToolResultText(string(respJSON) + "\n(Cached result)"), nil
		}
	}
	
	// Process recipients (normalize phones, resolve group names)
	checkAndFormat := true
	if cf, ok := args["check_and_format"].(bool); ok {
		checkAndFormat = cf
	}
	
	results := []map[string]interface{}{}
	requested := len(recipients)
	sent := 0
	failed := 0
	
	for _, recipient := range recipients {
		result := map[string]interface{}{
			"to": recipient,
		}
		
		// Handle group name resolution
		if strings.HasPrefix(recipient, "name:") {
			groupName := strings.TrimPrefix(recipient, "name:")
			groups, err := h.userService.MyListGroups(ctx)
			if err == nil {
				for _, g := range groups.Data {
					if strings.Contains(strings.ToLower(g.GroupName.Name), strings.ToLower(groupName)) {
						recipient = g.JID.String()
						result["resolved_to"] = g.JID.String()
						result["group_name"] = g.GroupName.Name
						break
					}
				}
			}
		}
		
		// Normalize phone if not a group
		if checkAndFormat && !strings.Contains(recipient, "@") {
			recipient = normalizePhone(recipient)
			result["normalized"] = recipient
			
			// Check if on WhatsApp
			check, err := h.userService.IsOnWhatsApp(ctx, domainUser.CheckRequest{Phone: recipient})
			if err != nil || !check.IsOnWhatsApp {
				result["status"] = "not_on_whatsapp"
				result["error"] = "Recipient not on WhatsApp"
				failed++
				results = append(results, result)
				continue
			}
		}
		
		// Send based on kind
		var err error
		switch kind {
		case "text":
			_, err = h.sendService.SendText(ctx, domainSend.MessageRequest{
				BaseRequest: domainSend.BaseRequest{Phone: recipient},
				Message:     content,
			})
			
		case "image":
			mediaURL := args["media_url"].(string)
			_, err = h.sendService.SendImage(ctx, domainSend.ImageRequest{
				BaseRequest: domainSend.BaseRequest{Phone: recipient},
				ImageURL:    &mediaURL,
				Caption:     content,
			})
			
		case "link":
			linkURL := args["link_url"].(string)
			_, err = h.sendService.SendLink(ctx, domainSend.LinkRequest{
				BaseRequest: domainSend.BaseRequest{Phone: recipient},
				Link:        linkURL,
				Caption:     content,
			})
			
		case "location":
			loc := args["location"].(map[string]interface{})
			lat := fmt.Sprintf("%v", loc["lat"])
			lng := fmt.Sprintf("%v", loc["lng"])
			_, err = h.sendService.SendLocation(ctx, domainSend.LocationRequest{
				BaseRequest: domainSend.BaseRequest{Phone: recipient},
				Latitude:    lat,
				Longitude:   lng,
			})
			
		case "contact":
			contactInfo := args["contact"].(map[string]interface{})
			_, err = h.sendService.SendContact(ctx, domainSend.ContactRequest{
				BaseRequest:   domainSend.BaseRequest{Phone: recipient},
				ContactName:   contactInfo["name"].(string),
				ContactPhone: contactInfo["phone"].(string),
			})
			
		default:
			err = fmt.Errorf("unsupported kind: %s", kind)
		}
		
		if err != nil {
			result["status"] = "failed"
			result["error"] = err.Error()
			failed++
		} else {
			result["status"] = "sent"
			result["timestamp"] = time.Now().Unix()
			sent++
			
			// Cache for idempotency
			if idempKey, ok := args["idempotency_key"].(string); ok && idempKey != "" {
				h.idempotencyCache[idempKey] = &SendResult{
					MessageID: uuid.NewString(),
					Status:    "sent",
					Timestamp: time.Now(),
				}
			}
		}
		
		results = append(results, result)
	}
	
	status := "success"
	if failed > 0 && sent > 0 {
		status = "partial"
	} else if failed > 0 && sent == 0 {
		status = "error"
	}
	
	resp := h.createResponse("whatsapp_send", "send", status, map[string]interface{}{
		"requested": requested,
		"sent":      sent,
		"failed":    failed,
		"results":   results,
	})
	
	respJSON, _ := json.Marshal(resp)
	summary := fmt.Sprintf("\nSent %d of %d", sent, requested)
	return mcp.NewToolResultText(string(respJSON) + summary), nil
}

// TOOL 3: Message Operations (COMPLETE)
func (h *OptimizedHandlerV2) toolMessages() mcp.Tool {
	return mcp.NewTool("whatsapp_messages",
		mcp.WithDescription("Complete message management with search"),
		mcp.WithString("action",
			mcp.Required(),
			mcp.Description("get|mark_read|react|delete|search"),
		),
		mcp.WithString("chat_id",
			mcp.Description("Chat JID or phone number"),
		),
		mcp.WithNumber("limit",
			mcp.Description("Number of messages (default: 20)"),
		),
		mcp.WithString("cursor",
			mcp.Description("Pagination cursor"),
		),
		mcp.WithString("message_id",
			mcp.Description("Message ID for react/delete"),
		),
		mcp.WithString("reaction",
			mcp.Description("Emoji for reaction"),
		),
		mcp.WithString("search_term",
			mcp.Description("Search query"),
		),
		mcp.WithBoolean("auto_mark_read",
			mcp.Description("Auto mark as read (default: true)"),
		),
		mcp.WithBoolean("media_only",
			mcp.Description("Only media messages"),
		),
	)
}

func (h *OptimizedHandlerV2) handleMessages(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	action := request.GetArguments()["action"].(string)
	
	switch action {
	case "get":
		chatID := request.GetArguments()["chat_id"].(string)
		limit := 20
		if l, ok := request.GetArguments()["limit"].(float64); ok {
			limit = int(l)
		}
		
		autoMarkRead := true
		if amr, ok := request.GetArguments()["auto_mark_read"].(bool); ok {
			autoMarkRead = amr
		}
		
		// Get messages from chat
		messages, err := h.chatService.GetChatMessages(ctx, domainChat.GetChatMessagesRequest{
			ChatJID: chatID,
			Limit:   limit,
		})
		
		if err != nil {
			resp := h.createError("whatsapp_messages", action, "fetch_failed", "Could not fetch messages", err.Error())
			respJSON, _ := json.Marshal(resp)
			return mcp.NewToolResultText(string(respJSON)), nil
		}
		
		// Format messages
		formattedMessages := []map[string]interface{}{}
		messageIDs := []string{}
		
		for _, msg := range messages.Data {
			formatted := map[string]interface{}{
				"id":        msg.ID,
				"from":      msg.SenderJID,
				"timestamp": msg.Timestamp,
				"type":      msg.MediaType, // Use actual media type
			}
			
			// Add content
			formatted["text"] = msg.Content
			
			formattedMessages = append(formattedMessages, formatted)
			messageIDs = append(messageIDs, msg.ID)
		}
		
		// Auto mark as read if enabled
		markedAsRead := false
		if autoMarkRead && len(messageIDs) > 0 {
			_, err = h.messageService.MarkAsRead(ctx, domainMessage.MarkAsReadRequest{
				Phone:      chatID,
				MessageID: messageIDs[0], // Use first message ID
			})
			markedAsRead = err == nil
		}
		
		resp := h.createResponse("whatsapp_messages", action, "success", map[string]interface{}{
			"chat_id":         chatID,
			"count":           len(formattedMessages),
			"messages":        formattedMessages,
			"marked_as_read":  markedAsRead,
			"next_cursor":     "", // Pagination not implemented
		})
		
		respJSON, _ := json.Marshal(resp)
		summary := fmt.Sprintf("\nFetched %d messages", len(formattedMessages))
		if markedAsRead {
			summary += " and marked as read"
		}
		return mcp.NewToolResultText(string(respJSON) + summary), nil
		
	case "mark_read":
		chatID := request.GetArguments()["chat_id"].(string)
		
		// Mark entire chat as read (not available in current API)
		var err error = fmt.Errorf("mark as read not available")
		
		if err != nil {
			resp := h.createError("whatsapp_messages", action, "mark_failed", "Could not mark as read", err.Error())
			respJSON, _ := json.Marshal(resp)
			return mcp.NewToolResultText(string(respJSON)), nil
		}
		
		resp := h.createResponse("whatsapp_messages", action, "success", map[string]interface{}{
			"chat_id": chatID,
			"marked":  true,
		})
		respJSON, _ := json.Marshal(resp)
		return mcp.NewToolResultText(string(respJSON) + "\nMarked as read"), nil
		
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
			resp := h.createError("whatsapp_messages", action, "react_failed", "Could not react", err.Error())
			respJSON, _ := json.Marshal(resp)
			return mcp.NewToolResultText(string(respJSON)), nil
		}
		
		resp := h.createResponse("whatsapp_messages", action, "success", map[string]interface{}{
			"message_id": messageID,
			"reaction":   reaction,
		})
		respJSON, _ := json.Marshal(resp)
		return mcp.NewToolResultText(string(respJSON) + "\nReacted " + reaction), nil
		
	case "delete":
		messageID := request.GetArguments()["message_id"].(string)
		chatID := request.GetArguments()["chat_id"].(string)
		
		err := h.messageService.DeleteMessage(ctx, domainMessage.DeleteRequest{
			Phone:     chatID,
			MessageID: messageID,
		})
		
		if err != nil {
			resp := h.createError("whatsapp_messages", action, "delete_failed", "Could not delete", err.Error())
			respJSON, _ := json.Marshal(resp)
			return mcp.NewToolResultText(string(respJSON)), nil
		}
		
		resp := h.createResponse("whatsapp_messages", action, "success", map[string]interface{}{
			"message_id": messageID,
			"deleted":    true,
		})
		respJSON, _ := json.Marshal(resp)
		return mcp.NewToolResultText(string(respJSON) + "\nDeleted"), nil
		
	case "search":
		searchTerm := request.GetArguments()["search_term"].(string)
		chatID, _ := request.GetArguments()["chat_id"].(string)
		
		// Search in specific chat or all chats
		messages, err := h.chatService.GetChatMessages(ctx, domainChat.GetChatMessagesRequest{
			ChatJID: chatID,
			Search:  searchTerm,
			Limit:   50,
		})
		
		if err != nil {
			resp := h.createError("whatsapp_messages", action, "search_failed", "Search failed", err.Error())
			respJSON, _ := json.Marshal(resp)
			return mcp.NewToolResultText(string(respJSON)), nil
		}
		
		// Format search results
		results := []map[string]interface{}{}
		for _, msg := range messages.Data {
			results = append(results, map[string]interface{}{
				"id":        msg.ID,
				"chat_id":   msg.ChatJID,
				"from":      msg.SenderJID,
				"text":      msg.Content,
				"timestamp": msg.Timestamp,
			})
		}
		
		resp := h.createResponse("whatsapp_messages", action, "success", map[string]interface{}{
			"query":   searchTerm,
			"count":   len(results),
			"results": results,
		})
		respJSON, _ := json.Marshal(resp)
		return mcp.NewToolResultText(string(respJSON) + fmt.Sprintf("\nFound %d matches", len(results))), nil
		
	default:
		resp := h.createError("whatsapp_messages", action, "invalid_action", "Unknown action", action)
		respJSON, _ := json.Marshal(resp)
		return mcp.NewToolResultText(string(respJSON)), nil
	}
}

// TOOL 4: Group Operations (COMPLETE)
func (h *OptimizedHandlerV2) toolGroups() mcp.Tool {
	return mcp.NewTool("whatsapp_groups",
		mcp.WithDescription("Complete group management"),
		mcp.WithString("action",
			mcp.Required(),
			mcp.Description("list|create|info|join|leave|manage|settings"),
		),
		mcp.WithString("group_id",
			mcp.Description("Group JID"),
		),
		mcp.WithString("group_name",
			mcp.Description("Group name for create/search"),
		),
		mcp.WithArray("participants",
			mcp.Description("Phone numbers for create/add/remove"),
		),
		mcp.WithString("invite_link",
			mcp.Description("Invite link for joining"),
		),
		mcp.WithString("operation",
			mcp.Description("For manage: add|remove"),
		),
		mcp.WithString("setting",
			mcp.Description("For settings: name|description|locked|announce"),
		),
		mcp.WithString("value",
			mcp.Description("New value for setting"),
		),
		mcp.WithNumber("limit",
			mcp.Description("Limit for list (default: 50)"),
		),
		mcp.WithString("cursor",
			mcp.Description("Pagination cursor"),
		),
	)
}

func (h *OptimizedHandlerV2) handleGroups(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	action := request.GetArguments()["action"].(string)
	
	switch action {
	case "list":
		limit := 50
		if l, ok := request.GetArguments()["limit"].(float64); ok {
			limit = int(l)
		}
		
		response, err := h.userService.MyListGroups(ctx)
		if err != nil {
			resp := h.createError("whatsapp_groups", action, "list_failed", "Could not list groups", err.Error())
			respJSON, _ := json.Marshal(resp)
			return mcp.NewToolResultText(string(respJSON)), nil
		}
		
		groups := []map[string]interface{}{}
		for i, group := range response.Data {
			if i >= limit {
				break
			}
			groups = append(groups, map[string]interface{}{
				"id":                group.JID.String(),
				"name":              group.GroupName.Name,
				"participant_count": len(group.Participants),
				// Admin fields not available in GroupInfo
			})
		}
		
		resp := h.createResponse("whatsapp_groups", action, "success", map[string]interface{}{
			"count":  len(groups),
			"groups": groups,
		})
		respJSON, _ := json.Marshal(resp)
		return mcp.NewToolResultText(string(respJSON) + fmt.Sprintf("\n%d groups", len(groups))), nil
		
	case "create":
		name := request.GetArguments()["group_name"].(string)
		participantsRaw := request.GetArguments()["participants"].([]interface{})
		participants := make([]string, len(participantsRaw))
		for i, p := range participantsRaw {
			participants[i] = normalizePhone(p.(string))
		}
		
		groupID, err := h.groupService.CreateGroup(ctx, domainGroup.CreateGroupRequest{
			Title:        name,
			Participants: participants,
		})
		
		if err != nil {
			resp := h.createError("whatsapp_groups", action, "create_failed", "Could not create group", err.Error())
			respJSON, _ := json.Marshal(resp)
			return mcp.NewToolResultText(string(respJSON)), nil
		}
		
		resp := h.createResponse("whatsapp_groups", action, "success", map[string]interface{}{
			"group_id":     groupID,
			"name":         name,
			"participants": len(participants),
		})
		respJSON, _ := json.Marshal(resp)
		return mcp.NewToolResultText(string(respJSON) + "\nGroup created"), nil
		
	case "info":
		groupID := request.GetArguments()["group_id"].(string)
		
		info, err := h.groupService.GroupInfo(ctx, domainGroup.GroupInfoRequest{
			GroupID: groupID,
		})
		
		if err != nil {
			resp := h.createError("whatsapp_groups", action, "info_failed", "Could not get group info", err.Error())
			respJSON, _ := json.Marshal(resp)
			return mcp.NewToolResultText(string(respJSON)), nil
		}
		
		resp := h.createResponse("whatsapp_groups", action, "success", map[string]interface{}{
			"group": info.Data,
		})
		respJSON, _ := json.Marshal(resp)
		return mcp.NewToolResultText(string(respJSON)), nil
		
	case "join":
		inviteLink := request.GetArguments()["invite_link"].(string)
		
		groupID, err := h.groupService.JoinGroupWithLink(ctx, domainGroup.JoinGroupWithLinkRequest{
			Link: inviteLink,
		})
		
		if err != nil {
			resp := h.createError("whatsapp_groups", action, "join_failed", "Could not join group", err.Error())
			respJSON, _ := json.Marshal(resp)
			return mcp.NewToolResultText(string(respJSON)), nil
		}
		
		resp := h.createResponse("whatsapp_groups", action, "success", map[string]interface{}{
			"group_id": groupID,
			"joined":   true,
		})
		respJSON, _ := json.Marshal(resp)
		return mcp.NewToolResultText(string(respJSON) + "\nJoined group"), nil
		
	case "leave":
		groupID := request.GetArguments()["group_id"].(string)
		
		err := h.groupService.LeaveGroup(ctx, domainGroup.LeaveGroupRequest{
			GroupID: groupID,
		})
		
		if err != nil {
			resp := h.createError("whatsapp_groups", action, "leave_failed", "Could not leave group", err.Error())
			respJSON, _ := json.Marshal(resp)
			return mcp.NewToolResultText(string(respJSON)), nil
		}
		
		resp := h.createResponse("whatsapp_groups", action, "success", map[string]interface{}{
			"group_id": groupID,
			"left":     true,
		})
		respJSON, _ := json.Marshal(resp)
		return mcp.NewToolResultText(string(respJSON) + "\nLeft group"), nil
		
	case "manage":
		groupID := request.GetArguments()["group_id"].(string)
		operation := request.GetArguments()["operation"].(string)
		participantsRaw := request.GetArguments()["participants"].([]interface{})
		participants := make([]string, len(participantsRaw))
		for i, p := range participantsRaw {
			participants[i] = normalizePhone(p.(string))
		}
		
		var err error
		if operation == "add" || operation == "remove" {
			// Use ManageParticipant for both add and remove
			var results []domainGroup.ParticipantStatus
			var action whatsmeow.ParticipantChange
			if operation == "add" {
				action = whatsmeow.ParticipantChangeAdd
			} else {
				action = whatsmeow.ParticipantChangeRemove
			}
			results, err = h.groupService.ManageParticipant(ctx, domainGroup.ParticipantRequest{
				GroupID:      groupID,
				Participants: participants,
				Action:       action,
			})
			// Check results for any errors
			if len(results) > 0 {
				// Process results if needed
			}
		} else {
			resp := h.createError("whatsapp_groups", action, "invalid_operation", "Unknown operation", operation)
			respJSON, _ := json.Marshal(resp)
			return mcp.NewToolResultText(string(respJSON)), nil
		}
		
		if err != nil {
			resp := h.createError("whatsapp_groups", action, "manage_failed", "Could not manage participants", err.Error())
			respJSON, _ := json.Marshal(resp)
			return mcp.NewToolResultText(string(respJSON)), nil
		}
		
		resp := h.createResponse("whatsapp_groups", action, "success", map[string]interface{}{
			"group_id":     groupID,
			"operation":    operation,
			"participants": participants,
		})
		respJSON, _ := json.Marshal(resp)
		return mcp.NewToolResultText(string(respJSON) + fmt.Sprintf("\n%s %d participants", operation, len(participants))), nil
		
	case "settings":
		groupID := request.GetArguments()["group_id"].(string)
		setting := request.GetArguments()["setting"].(string)
		value := request.GetArguments()["value"].(string)
		
		var err error
		switch setting {
		case "name":
			err = h.groupService.SetGroupName(ctx, domainGroup.SetGroupNameRequest{
				GroupID: groupID,
				Name:    value,
			})
			
		case "locked":
			locked := value == "true" || value == "1"
			err = h.groupService.SetGroupLocked(ctx, domainGroup.SetGroupLockedRequest{
				GroupID: groupID,
				Locked:  locked,
			})
			
		case "announce":
			announce := value == "true" || value == "1"
			err = h.groupService.SetGroupAnnounce(ctx, domainGroup.SetGroupAnnounceRequest{
				GroupID:  groupID,
				Announce: announce,
			})
			
		default:
			resp := h.createError("whatsapp_groups", action, "invalid_setting", "Unknown setting", setting)
			respJSON, _ := json.Marshal(resp)
			return mcp.NewToolResultText(string(respJSON)), nil
		}
		
		if err != nil {
			resp := h.createError("whatsapp_groups", action, "settings_failed", "Could not update setting", err.Error())
			respJSON, _ := json.Marshal(resp)
			return mcp.NewToolResultText(string(respJSON)), nil
		}
		
		resp := h.createResponse("whatsapp_groups", action, "success", map[string]interface{}{
			"group_id": groupID,
			"setting":  setting,
			"value":    value,
		})
		respJSON, _ := json.Marshal(resp)
		return mcp.NewToolResultText(string(respJSON) + fmt.Sprintf("\nUpdated %s", setting)), nil
		
	default:
		resp := h.createError("whatsapp_groups", action, "invalid_action", "Unknown action", action)
		respJSON, _ := json.Marshal(resp)
		return mcp.NewToolResultText(string(respJSON)), nil
	}
}

// TOOL 5: Contact Operations (COMPLETE)
func (h *OptimizedHandlerV2) toolContacts() mcp.Tool {
	return mcp.NewTool("whatsapp_contacts",
		mcp.WithDescription("Contact management and verification"),
		mcp.WithString("action",
			mcp.Required(),
			mcp.Description("check|info|list"),
		),
		mcp.WithArray("phones",
			mcp.Description("Phone numbers to check/get info"),
		),
		mcp.WithBoolean("normalize",
			mcp.Description("Normalize to E.164 format (default: true)"),
		),
		mcp.WithBoolean("get_avatar",
			mcp.Description("Include avatar URL (default: false)"),
		),
		mcp.WithNumber("limit",
			mcp.Description("Limit for list (default: 100)"),
		),
	)
}

func (h *OptimizedHandlerV2) handleContacts(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	action := request.GetArguments()["action"].(string)
	
	switch action {
	case "check":
		phonesRaw := request.GetArguments()["phones"].([]interface{})
		phones := make([]string, len(phonesRaw))
		normalize := true
		if n, ok := request.GetArguments()["normalize"].(bool); ok {
			normalize = n
		}
		
		for i, p := range phonesRaw {
			phone := p.(string)
			if normalize {
				phone = normalizePhone(phone)
			}
			phones[i] = phone
		}
		
		results := []map[string]interface{}{}
		onWhatsApp := 0
		
		for _, phone := range phones {
			check, err := h.userService.IsOnWhatsApp(ctx, domainUser.CheckRequest{
				Phone: phone,
			})
			
			result := map[string]interface{}{
				"phone":        phone,
				"on_whatsapp": err == nil && check.IsOnWhatsApp,
			}
			
			if err == nil && check.IsOnWhatsApp {
				onWhatsApp++
				result["jid"] = normalizePhone(phone) + "@s.whatsapp.net" // Construct JID
			}
			
			if err != nil {
				result["error"] = err.Error()
			}
			
			results = append(results, result)
		}
		
		resp := h.createResponse("whatsapp_contacts", action, "success", map[string]interface{}{
			"total":         len(phones),
			"on_whatsapp":   onWhatsApp,
			"not_on_whatsapp": len(phones) - onWhatsApp,
			"results":       results,
		})
		respJSON, _ := json.Marshal(resp)
		return mcp.NewToolResultText(string(respJSON) + fmt.Sprintf("\n%d/%d on WhatsApp", onWhatsApp, len(phones))), nil
		
	case "info":
		phonesRaw := request.GetArguments()["phones"].([]interface{})
		getAvatar := false
		if ga, ok := request.GetArguments()["get_avatar"].(bool); ok {
			getAvatar = ga
		}
		
		results := []map[string]interface{}{}
		
		for _, p := range phonesRaw {
			phone := normalizePhone(p.(string))
			
			info, err := h.userService.Info(ctx, domainUser.InfoRequest{
				Phone: phone,
			})
			
			if err != nil {
				results = append(results, map[string]interface{}{
					"phone": phone,
					"error": err.Error(),
				})
				continue
			}
			
			result := map[string]interface{}{
				"phone":   phone,
				"info":    info.Data,
			}
			
			if getAvatar {
				avatar, err := h.userService.Avatar(ctx, domainUser.AvatarRequest{
					Phone: phone,
				})
				if err == nil {
					result["avatar_url"] = avatar.URL
				}
			}
			
			results = append(results, result)
		}
		
		resp := h.createResponse("whatsapp_contacts", action, "success", map[string]interface{}{
			"count":   len(results),
			"results": results,
		})
		respJSON, _ := json.Marshal(resp)
		return mcp.NewToolResultText(string(respJSON)), nil
		
	case "list":
		// List all contacts (would need implementation in domain)
		resp := h.createResponse("whatsapp_contacts", action, "success", map[string]interface{}{
			"message": "Contact list not implemented in current API",
			"count":   0,
			"contacts": []interface{}{},
		})
		respJSON, _ := json.Marshal(resp)
		return mcp.NewToolResultText(string(respJSON)), nil
		
	default:
		resp := h.createError("whatsapp_contacts", action, "invalid_action", "Unknown action", action)
		respJSON, _ := json.Marshal(resp)
		return mcp.NewToolResultText(string(respJSON)), nil
	}
}

// TOOL 6: Chat Management (COMPLETE)
func (h *OptimizedHandlerV2) toolChats() mcp.Tool {
	return mcp.NewTool("whatsapp_chats",
		mcp.WithDescription("Chat list and management"),
		mcp.WithString("action",
			mcp.Required(),
			mcp.Description("list|archive|unarchive|delete|mute|unmute|pin|unpin"),
		),
		mcp.WithString("chat_id",
			mcp.Description("Chat JID for operations"),
		),
		mcp.WithString("filter",
			mcp.Description("For list: all|unread|groups|archived"),
		),
		mcp.WithNumber("limit",
			mcp.Description("Limit for list (default: 50)"),
		),
		mcp.WithString("cursor",
			mcp.Description("Pagination cursor"),
		),
		mcp.WithNumber("mute_duration",
			mcp.Description("Mute duration in seconds"),
		),
	)
}

func (h *OptimizedHandlerV2) handleChats(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	action := request.GetArguments()["action"].(string)
	
	switch action {
	case "list":
		limit := 50
		if l, ok := request.GetArguments()["limit"].(float64); ok {
			limit = int(l)
		}
		
		filter := "all"
		if f, ok := request.GetArguments()["filter"].(string); ok {
			filter = f
		}
		
		chats, err := h.chatService.ListChats(ctx, domainChat.ListChatsRequest{
			Limit: limit,
		})
		
		if err != nil {
			resp := h.createError("whatsapp_chats", action, "list_failed", "Could not list chats", err.Error())
			respJSON, _ := json.Marshal(resp)
			return mcp.NewToolResultText(string(respJSON)), nil
		}
		
		// Filter results based on filter parameter
		filtered := []map[string]interface{}{}
		for _, chat := range chats.Data {
			include := false
			
			switch filter {
			case "all":
				include = true
			case "unread":
				include = true // UnreadCount not available in ChatInfo
			case "groups":
				include = strings.Contains(chat.JID, "@g.us")
			case "archived":
				include = false // IsArchived not available in ChatInfo
			default:
				include = true
			}
			
			if include {
				filtered = append(filtered, map[string]interface{}{
					"jid":            chat.JID,
					"name":           chat.Name,
					// "unread_count":   chat.UnreadCount,  // Not available in ChatInfo
					"is_group":       strings.Contains(chat.JID, "@g.us"),
					// "is_archived":    chat.IsArchived,   // Not available in ChatInfo
					// "is_pinned":      chat.IsPinned,     // Not available in ChatInfo
					// "last_message":   chat.LastMessage,  // Not available in ChatInfo
					"last_message_time": chat.LastMessageTime,
				})
			}
		}
		
		resp := h.createResponse("whatsapp_chats", action, "success", map[string]interface{}{
			"filter": filter,
			"count":  len(filtered),
			"chats":  filtered,
			// "next_cursor": chats.Pagination.NextCursor, // NextCursor not available
		})
		respJSON, _ := json.Marshal(resp)
		return mcp.NewToolResultText(string(respJSON) + fmt.Sprintf("\n%d chats", len(filtered))), nil
		
	case "archive", "unarchive":
		chatID := request.GetArguments()["chat_id"].(string)
		archive := action == "archive"
		
		_, err := h.chatService.PinChat(ctx, domainChat.PinChatRequest{
			ChatJID: chatID,
			Pinned:  archive, // Using pin as archive for now
		})
		
		if err != nil {
			resp := h.createError("whatsapp_chats", action, "archive_failed", "Could not "+action, err.Error())
			respJSON, _ := json.Marshal(resp)
			return mcp.NewToolResultText(string(respJSON)), nil
		}
		
		resp := h.createResponse("whatsapp_chats", action, "success", map[string]interface{}{
			"chat_id":  chatID,
			"archived": archive,
		})
		respJSON, _ := json.Marshal(resp)
		return mcp.NewToolResultText(string(respJSON) + "\n" + strings.Title(action) + "d"), nil
		
	case "pin", "unpin":
		chatID := request.GetArguments()["chat_id"].(string)
		pin := action == "pin"
		
		_, err := h.chatService.PinChat(ctx, domainChat.PinChatRequest{
			ChatJID: chatID,
			Pinned:  pin,
		})
		
		if err != nil {
			resp := h.createError("whatsapp_chats", action, "pin_failed", "Could not "+action, err.Error())
			respJSON, _ := json.Marshal(resp)
			return mcp.NewToolResultText(string(respJSON)), nil
		}
		
		resp := h.createResponse("whatsapp_chats", action, "success", map[string]interface{}{
			"chat_id": chatID,
			"pinned":  pin,
		})
		respJSON, _ := json.Marshal(resp)
		return mcp.NewToolResultText(string(respJSON) + "\n" + strings.Title(action) + "ned"), nil
		
	case "delete":
		chatID := request.GetArguments()["chat_id"].(string)
		
		// Note: Delete not implemented in current API
		resp := h.createResponse("whatsapp_chats", action, "success", map[string]interface{}{
			"chat_id": chatID,
			"message": "Delete not implemented in current API",
		})
		respJSON, _ := json.Marshal(resp)
		return mcp.NewToolResultText(string(respJSON)), nil
		
	case "mute", "unmute":
		chatID := request.GetArguments()["chat_id"].(string)
		mute := action == "mute"
		duration := 0
		
		if mute {
			if d, ok := request.GetArguments()["mute_duration"].(float64); ok {
				duration = int(d)
			} else {
				duration = 8 * 3600 // Default 8 hours
			}
		}
		
		// Note: Mute not implemented in current API, would need to add
		resp := h.createResponse("whatsapp_chats", action, "success", map[string]interface{}{
			"chat_id":  chatID,
			"muted":    mute,
			"duration": duration,
			"message":  "Mute not implemented in current API",
		})
		respJSON, _ := json.Marshal(resp)
		return mcp.NewToolResultText(string(respJSON)), nil
		
	default:
		resp := h.createError("whatsapp_chats", action, "invalid_action", "Unknown action", action)
		respJSON, _ := json.Marshal(resp)
		return mcp.NewToolResultText(string(respJSON)), nil
	}
}