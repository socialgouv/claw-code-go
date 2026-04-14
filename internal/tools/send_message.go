package tools

import (
	"claw-code-go/internal/api"
	"encoding/json"
	"fmt"
	"time"
)

func SendUserMessageTool() api.Tool {
	return api.Tool{
		Name:        "send_user_message",
		Description: "Send a message to the user with optional attachments.",
		InputSchema: api.InputSchema{
			Type: "object",
			Properties: map[string]api.Property{
				"message": {Type: "string", Description: "The message text to send."},
				"status":  {Type: "string", Description: "Message status: normal or proactive."},
			},
			Required: []string{"message", "status"},
		},
	}
}

func ExecuteSendUserMessage(input map[string]any) (string, error) {
	message, ok := input["message"].(string)
	if !ok || message == "" {
		return "", fmt.Errorf("send_user_message: 'message' is required and must not be empty")
	}
	status, _ := input["status"].(string)
	if status == "" {
		status = "normal"
	}
	result := map[string]any{
		"message":     message,
		"status":      status,
		"sent_at":     time.Now().UTC().Format(time.RFC3339),
		"attachments": []any{},
	}
	out, _ := json.MarshalIndent(result, "", "  ")
	return string(out), nil
}
