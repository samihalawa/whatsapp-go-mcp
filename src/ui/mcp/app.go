package mcp

import (
	"context"
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"strings"

	domainApp "github.com/aldinokemal/go-whatsapp-web-multidevice/domains/app"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/skip2/go-qrcode"
)

type AppHandler struct {
	appService domainApp.IAppUsecase
}

func InitMcpApp(appService domainApp.IAppUsecase) *AppHandler {
	return &AppHandler{
		appService: appService,
	}
}

func (a *AppHandler) AddAppTools(mcpServer *server.MCPServer) {
	mcpServer.AddTool(a.toolGetQR(), a.handleGetQR)
	mcpServer.AddTool(a.toolLoginWithCode(), a.handleLoginWithCode)
	mcpServer.AddTool(a.toolLogout(), a.handleLogout)
	mcpServer.AddTool(a.toolReconnect(), a.handleReconnect)
	mcpServer.AddTool(a.toolGetDevices(), a.handleGetDevices)
}

func (a *AppHandler) toolGetQR() mcp.Tool {
	return mcp.NewTool("whatsapp_get_qr",
		mcp.WithDescription("Get WhatsApp QR code for login. Returns the QR code image path and code string."),
	)
}

func (a *AppHandler) handleGetQR(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	res, err := a.appService.Login(ctx)
	if err != nil {
		return nil, err
	}

	// Generate QR code as base64 for remote display
	qrPNG, err := qrcode.Encode(res.Code, qrcode.Medium, 512)
	if err != nil {
		// Fall back to text if QR generation fails
		result := fmt.Sprintf("QR Code (text format):\n%s\n\nDuration: %v seconds\n\nOpen WhatsApp on your phone > Settings > Linked Devices > Link a Device and scan this code", 
			res.Code, res.Duration)
		return mcp.NewToolResultText(result), nil
	}

	// Create base64 data URI for the QR code
	base64QR := base64.StdEncoding.EncodeToString(qrPNG)
	dataURI := fmt.Sprintf("data:image/png;base64,%s", base64QR)

	// Return both the data URI and the raw code as fallback
	result := fmt.Sprintf("QR Code ready for scanning!\n\nData URI (copy and paste in browser):\n%s\n\nAlternative - Raw QR Code:\n%s\n\nDuration: %v seconds\n\nTo use:\n1. Copy the Data URI above and paste in a browser to see the QR code\n2. Or use the raw code with any QR generator\n3. Open WhatsApp > Settings > Linked Devices > Link a Device\n4. Scan the QR code", 
		dataURI, res.Code, res.Duration)
	
	// Also try to read the file if it exists for local use
	if res.ImagePath != "" && !strings.HasPrefix(res.ImagePath, "data:") {
		if fileData, err := ioutil.ReadFile(res.ImagePath); err == nil {
			base64File := base64.StdEncoding.EncodeToString(fileData)
			result = fmt.Sprintf("%s\n\nLocal file also available at: %s", result, res.ImagePath)
			_ = base64File // We have it if needed
		}
	}
	
	return mcp.NewToolResultText(result), nil
}

func (a *AppHandler) toolLoginWithCode() mcp.Tool {
	return mcp.NewTool("whatsapp_login_with_code",
		mcp.WithDescription("Login to WhatsApp using phone number code pairing."),
		mcp.WithString("phone_number",
			mcp.Required(),
			mcp.Description("Phone number with country code (e.g., +1234567890)"),
		),
	)
}

func (a *AppHandler) handleLoginWithCode(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	phoneNumber, ok := request.GetArguments()["phone_number"].(string)
	if !ok {
		return nil, fmt.Errorf("phone_number must be a string")
	}

	code, err := a.appService.LoginWithCode(ctx, phoneNumber)
	if err != nil {
		return nil, err
	}

	return mcp.NewToolResultText(fmt.Sprintf("Login code generated: %s", code)), nil
}

func (a *AppHandler) toolLogout() mcp.Tool {
	return mcp.NewTool("whatsapp_logout",
		mcp.WithDescription("Logout from WhatsApp and clear session."),
	)
}

func (a *AppHandler) handleLogout(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	err := a.appService.Logout(ctx)
	if err != nil {
		return nil, err
	}

	return mcp.NewToolResultText("Successfully logged out from WhatsApp"), nil
}

func (a *AppHandler) toolReconnect() mcp.Tool {
	return mcp.NewTool("whatsapp_reconnect",
		mcp.WithDescription("Reconnect to WhatsApp server."),
	)
}

func (a *AppHandler) handleReconnect(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	err := a.appService.Reconnect(ctx)
	if err != nil {
		return nil, err
	}

	return mcp.NewToolResultText("Successfully reconnected to WhatsApp"), nil
}

func (a *AppHandler) toolGetDevices() mcp.Tool {
	return mcp.NewTool("whatsapp_get_devices",
		mcp.WithDescription("Get list of connected WhatsApp devices."),
	)
}

func (a *AppHandler) handleGetDevices(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	devices, err := a.appService.FetchDevices(ctx)
	if err != nil {
		return nil, err
	}

	result := "Connected devices:\n"
	for i, device := range devices {
		result += fmt.Sprintf("%d. %s (%s)\n", i+1, device.Name, device.Device)
	}

	return mcp.NewToolResultText(result), nil
}