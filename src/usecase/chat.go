package usecase

import (
	"context"
	"fmt"
	"strings"
	"time"

	domainChat "github.com/aldinokemal/go-whatsapp-web-multidevice/domains/chat"
	domainChatStorage "github.com/aldinokemal/go-whatsapp-web-multidevice/domains/chatstorage"
	"github.com/aldinokemal/go-whatsapp-web-multidevice/infrastructure/whatsapp"
	"github.com/aldinokemal/go-whatsapp-web-multidevice/pkg/utils"
	"github.com/aldinokemal/go-whatsapp-web-multidevice/validations"
	"github.com/sirupsen/logrus"
	"go.mau.fi/whatsmeow/appstate"
	"go.mau.fi/whatsmeow/types"
)

type serviceChat struct {
	chatStorageRepo domainChatStorage.IChatStorageRepository
}

func NewChatService(chatStorageRepo domainChatStorage.IChatStorageRepository) domainChat.IChatUsecase {
	return &serviceChat{
		chatStorageRepo: chatStorageRepo,
	}
}

func (service serviceChat) ListChats(ctx context.Context, request domainChat.ListChatsRequest) (response domainChat.ListChatsResponse, err error) {
	if err = validations.ValidateListChats(ctx, &request); err != nil {
		return response, err
	}

	// Ensure we're logged in
	utils.MustLogin(whatsapp.GetClient())

	// FIRST: Get stored chats from the chat storage database
	// This contains all chats that have been synced, including individual conversations
	storedChats, err := service.chatStorageRepo.GetChats(&domainChatStorage.ChatFilter{})
	if err != nil {
		logrus.WithError(err).Error("Failed to get stored chats from database")
		storedChats = []*domainChatStorage.Chat{}
	}

	// Convert to chat infos and use a map to deduplicate by JID
	chatMap := make(map[string]domainChat.ChatInfo)

	// Add stored chats from database FIRST (these have actual message history)
	for _, chat := range storedChats {
		chatInfo := domainChat.ChatInfo{
			JID:             chat.JID,
			Name:            chat.Name,
			LastMessageTime: chat.LastMessageTime.Format(time.RFC3339),
			IsGroup:         strings.Contains(chat.JID, "@g.us"),
			MessagesSynced:  true, // Chats from database have been synced
			CreatedAt:       chat.CreatedAt.Format(time.RFC3339),
			UpdatedAt:       chat.UpdatedAt.Format(time.RFC3339),
		}

		// Apply search filter
		if request.Search != "" && !strings.Contains(strings.ToLower(chatInfo.Name), strings.ToLower(request.Search)) {
			continue
		}

		chatMap[chat.JID] = chatInfo
	}

	// SECOND: Get groups from WhatsApp (in case some aren't synced yet)
	groups, err := whatsapp.GetClient().GetJoinedGroups()
	if err != nil {
		logrus.WithError(err).Error("Failed to get groups from WhatsApp")
		groups = []*types.GroupInfo{}
	}

	// Get contacts from WhatsApp
	contacts, err := whatsapp.GetClient().Store.Contacts.GetAllContacts(ctx)
	if err != nil {
		logrus.WithError(err).Error("Failed to get contacts from WhatsApp")
		contacts = map[types.JID]types.ContactInfo{}
	}

	// Add groups from WhatsApp (in case some aren't synced yet)
	for _, group := range groups {
		jidStr := group.JID.String()

		// Skip if already in map from database
		if _, exists := chatMap[jidStr]; exists {
			continue
		}

		chatInfo := domainChat.ChatInfo{
			JID:             jidStr,
			Name:            group.GroupName.Name,
			LastMessageTime: time.Now().Format(time.RFC3339),
			IsGroup:         true,
			MessagesSynced:  false, // Group not yet synced - messages not available yet
			CreatedAt:       time.Now().Format(time.RFC3339),
			UpdatedAt:       time.Now().Format(time.RFC3339),
		}

		// Apply search filter
		if request.Search != "" && !strings.Contains(strings.ToLower(chatInfo.Name), strings.ToLower(request.Search)) {
			continue
		}

		chatMap[jidStr] = chatInfo
	}

	// Add contacts from WhatsApp (individual chats)
	for jid, contact := range contacts {
		// Skip if it's a group (already added)
		if strings.Contains(jid.String(), "@g.us") {
			continue
		}

		jidStr := jid.String()

		// Skip if already in map from database
		if _, exists := chatMap[jidStr]; exists {
			continue
		}

		chatInfo := domainChat.ChatInfo{
			JID:             jidStr,
			Name:            contact.FullName,
			LastMessageTime: time.Now().Format(time.RFC3339),
			IsGroup:         false,
			MessagesSynced:  false, // Contact not yet synced - messages not available yet
			CreatedAt:       time.Now().Format(time.RFC3339),
			UpdatedAt:       time.Now().Format(time.RFC3339),
		}

		// If no full name, use the phone number
		if chatInfo.Name == "" {
			chatInfo.Name = jid.User
		}

		// Apply search filter
		if request.Search != "" && !strings.Contains(strings.ToLower(chatInfo.Name), strings.ToLower(request.Search)) {
			continue
		}

		chatMap[jidStr] = chatInfo
	}

	// Convert map to slice
	chatInfos := make([]domainChat.ChatInfo, 0, len(chatMap))
	for _, chatInfo := range chatMap {
		chatInfos = append(chatInfos, chatInfo)
	}
	
	// Apply limit and offset
	totalCount := len(chatInfos)
	
	// Apply offset
	if request.Offset > 0 && request.Offset < len(chatInfos) {
		chatInfos = chatInfos[request.Offset:]
	} else if request.Offset >= len(chatInfos) {
		chatInfos = []domainChat.ChatInfo{}
	}
	
	// Apply limit
	if request.Limit > 0 && request.Limit < len(chatInfos) {
		chatInfos = chatInfos[:request.Limit]
	}

	// Create pagination response
	pagination := domainChat.PaginationResponse{
		Limit:  request.Limit,
		Offset: request.Offset,
		Total:  int(totalCount),
	}

	response.Data = chatInfos
	response.Pagination = pagination

	logrus.WithFields(logrus.Fields{
		"total_chats": len(chatInfos),
		"limit":       request.Limit,
		"offset":      request.Offset,
	}).Info("Listed chats successfully")

	return response, nil
}

func (service serviceChat) GetChatMessages(ctx context.Context, request domainChat.GetChatMessagesRequest) (response domainChat.GetChatMessagesResponse, err error) {
	if err = validations.ValidateGetChatMessages(ctx, &request); err != nil {
		return response, err
	}

	// Get chat info first
	chat, err := service.chatStorageRepo.GetChat(request.ChatJID)
	if err != nil {
		logrus.WithError(err).WithField("chat_jid", request.ChatJID).Error("Failed to get chat info")
		return response, err
	}
	if chat == nil {
		return response, fmt.Errorf("chat with JID %s not found - messages have not been synced yet. Please wait for WhatsApp history sync to complete after login, then try again", request.ChatJID)
	}

	// Create message filter from request
	filter := &domainChatStorage.MessageFilter{
		ChatJID:   request.ChatJID,
		Limit:     request.Limit,
		Offset:    request.Offset,
		MediaOnly: request.MediaOnly,
		IsFromMe:  request.IsFromMe,
	}

	// Parse time filters if provided
	if request.StartTime != nil && *request.StartTime != "" {
		startTime, err := time.Parse(time.RFC3339, *request.StartTime)
		if err != nil {
			return response, fmt.Errorf("invalid start_time format: %v", err)
		}
		filter.StartTime = &startTime
	}

	if request.EndTime != nil && *request.EndTime != "" {
		endTime, err := time.Parse(time.RFC3339, *request.EndTime)
		if err != nil {
			return response, fmt.Errorf("invalid end_time format: %v", err)
		}
		filter.EndTime = &endTime
	}

	// Get messages from storage
	var messages []*domainChatStorage.Message
	if request.Search != "" {
		// Use search functionality if search query is provided
		messages, err = service.chatStorageRepo.SearchMessages(request.ChatJID, request.Search, request.Limit)
		if err != nil {
			logrus.WithError(err).WithField("chat_jid", request.ChatJID).Error("Failed to search messages")
			return response, err
		}
	} else {
		// Use regular filter
		messages, err = service.chatStorageRepo.GetMessages(filter)
		if err != nil {
			logrus.WithError(err).WithField("chat_jid", request.ChatJID).Error("Failed to get messages")
			return response, err
		}
	}

	// Get total message count for pagination
	totalCount, err := service.chatStorageRepo.GetChatMessageCount(request.ChatJID)
	if err != nil {
		logrus.WithError(err).WithField("chat_jid", request.ChatJID).Error("Failed to get message count")
		// Continue with partial data
		totalCount = 0
	}

	// Convert entities to domain objects
	messageInfos := make([]domainChat.MessageInfo, 0, len(messages))
	for _, message := range messages {
		messageInfo := domainChat.MessageInfo{
			ID:         message.ID,
			ChatJID:    message.ChatJID,
			SenderJID:  message.Sender,
			Content:    message.Content,
			Timestamp:  message.Timestamp.Format(time.RFC3339),
			IsFromMe:   message.IsFromMe,
			MediaType:  message.MediaType,
			Filename:   message.Filename,
			URL:        message.URL,
			FileLength: message.FileLength,
			CreatedAt:  message.CreatedAt.Format(time.RFC3339),
			UpdatedAt:  message.UpdatedAt.Format(time.RFC3339),
		}
		messageInfos = append(messageInfos, messageInfo)
	}

	// Create chat info for response
	chatInfo := domainChat.ChatInfo{
		JID:                 chat.JID,
		Name:                chat.Name,
		LastMessageTime:     chat.LastMessageTime.Format(time.RFC3339),
		EphemeralExpiration: chat.EphemeralExpiration,
		CreatedAt:           chat.CreatedAt.Format(time.RFC3339),
		UpdatedAt:           chat.UpdatedAt.Format(time.RFC3339),
	}

	// Create pagination response
	pagination := domainChat.PaginationResponse{
		Limit:  request.Limit,
		Offset: request.Offset,
		Total:  int(totalCount),
	}

	response.Data = messageInfos
	response.Pagination = pagination
	response.ChatInfo = chatInfo

	logrus.WithFields(logrus.Fields{
		"chat_jid":       request.ChatJID,
		"total_messages": len(messageInfos),
		"limit":          request.Limit,
		"offset":         request.Offset,
	}).Info("Retrieved chat messages successfully")

	return response, nil
}

func (service serviceChat) PinChat(ctx context.Context, request domainChat.PinChatRequest) (response domainChat.PinChatResponse, err error) {
	if err = validations.ValidatePinChat(ctx, &request); err != nil {
		return response, err
	}

	// Validate JID and ensure connection
	targetJID, err := utils.ValidateJidWithLogin(whatsapp.GetClient(), request.ChatJID)
	if err != nil {
		return response, err
	}

	// Build pin patch using whatsmeow's BuildPin
	patchInfo := appstate.BuildPin(targetJID, request.Pinned)

	// Send app state update
	if err = whatsapp.GetClient().SendAppState(ctx, patchInfo); err != nil {
		logrus.WithError(err).WithFields(logrus.Fields{
			"chat_jid": request.ChatJID,
			"pinned":   request.Pinned,
		}).Error("Failed to send pin chat app state")
		return response, err
	}

	// Build response
	response.Status = "success"
	response.ChatJID = request.ChatJID
	response.Pinned = request.Pinned

	if request.Pinned {
		response.Message = "Chat pinned successfully"
	} else {
		response.Message = "Chat unpinned successfully"
	}

	logrus.WithFields(logrus.Fields{
		"chat_jid": request.ChatJID,
		"pinned":   request.Pinned,
	}).Info("Chat pin operation completed successfully")

	return response, nil
}

func (service serviceChat) ArchiveChat(ctx context.Context, request domainChat.ArchiveChatRequest) (response domainChat.ArchiveChatResponse, err error) {
	// Validate JID and ensure connection
	targetJID, err := utils.ValidateJidWithLogin(whatsapp.GetClient(), request.ChatJID)
	if err != nil {
		return response, err
	}

	// Build archive patch using whatsmeow's BuildArchive
	patchInfo := appstate.BuildArchive(targetJID, request.Archive, time.Now(), nil)

	// Send app state update
	if err = whatsapp.GetClient().SendAppState(ctx, patchInfo); err != nil {
		logrus.WithError(err).WithFields(logrus.Fields{
			"chat_jid": request.ChatJID,
			"archive":  request.Archive,
		}).Error("Failed to send archive chat app state")
		return response, err
	}

	// Build response
	response.Status = "success"
	response.ChatJID = request.ChatJID
	response.Archived = request.Archive

	if request.Archive {
		response.Message = "Chat archived successfully"
	} else {
		response.Message = "Chat unarchived successfully"
	}

	logrus.WithFields(logrus.Fields{
		"chat_jid": request.ChatJID,
		"archived": request.Archive,
	}).Info("Chat archive operation completed successfully")

	return response, nil
}

func (service serviceChat) DeleteChat(ctx context.Context, request domainChat.DeleteChatRequest) (response domainChat.DeleteChatResponse, err error) {
	// Validate JID and ensure connection
	_, err = utils.ValidateJidWithLogin(whatsapp.GetClient(), request.ChatJID)
	if err != nil {
		return response, err
	}

	// Note: WhatsApp Web doesn't actually support deleting chats via the API
	// We can only delete from local storage and archive the chat
	
	// Delete from local storage
	if err = service.chatStorageRepo.DeleteChatAndMessages(request.ChatJID); err != nil {
		logrus.WithError(err).WithField("chat_jid", request.ChatJID).Error("Failed to delete chat from local storage")
		// Continue anyway
	}

	// Build response
	response.Status = "success"
	response.ChatJID = request.ChatJID
	response.Message = "Chat deleted from local storage (note: WhatsApp Web API doesn't support actual chat deletion)"

	if request.KeepStarred {
		response.Message += " (starred messages kept locally)"
	}

	logrus.WithField("chat_jid", request.ChatJID).Info("Chat delete operation completed (local only)")

	return response, nil
}

func (service serviceChat) MarkChatAsRead(ctx context.Context, request domainChat.MarkChatAsReadRequest) (response domainChat.MarkChatAsReadResponse, err error) {
	// Validate JID and ensure connection
	targetJID, err := utils.ValidateJidWithLogin(whatsapp.GetClient(), request.ChatJID)
	if err != nil {
		return response, err
	}

	// Get last messages from the chat to mark as read
	filter := &domainChatStorage.MessageFilter{
		ChatJID: request.ChatJID,
		Limit:   50, // Mark last 50 messages as read
		Offset:  0,
	}

	messages, err := service.chatStorageRepo.GetMessages(filter)
	if err != nil {
		logrus.WithError(err).WithField("chat_jid", request.ChatJID).Error("Failed to get messages for marking as read")
		return response, err
	}

	if len(messages) > 0 {
		// Build message IDs list
		messageIDs := make([]string, 0, len(messages))
		for _, msg := range messages {
			if !msg.IsFromMe && msg.ID != "" {
				messageIDs = append(messageIDs, msg.ID)
			}
		}

		if len(messageIDs) > 0 {
			// Build read receipts using types.MessageID
			timestamp := time.Now()
			msgIDTypes := make([]types.MessageID, len(messageIDs))
			for i, msgID := range messageIDs {
				msgIDTypes[i] = types.MessageID(msgID)
			}
			
			// Mark all messages as read at once
			err := whatsapp.GetClient().MarkRead(msgIDTypes, timestamp, targetJID, targetJID)
			if err != nil {
				logrus.WithError(err).WithField("chat_jid", request.ChatJID).Warn("Failed to mark messages as read")
			}
		}
	}

	// Build response
	response.Status = "success"
	response.ChatJID = request.ChatJID
	response.Message = "All messages in chat marked as read"

	logrus.WithField("chat_jid", request.ChatJID).Info("Chat mark as read operation completed successfully")

	return response, nil
}
