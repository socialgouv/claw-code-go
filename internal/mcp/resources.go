package mcp

import "context"

// McpResourceInfo describes an MCP resource.
type McpResourceInfo struct {
	URI         string `json:"uri"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	MimeType    string `json:"mimeType,omitempty"`
}

// ListResources queries the MCP server for available resources.
func (c *Client) ListResources(ctx context.Context) ([]McpResourceInfo, error) {
	var result struct {
		Resources []McpResourceInfo `json:"resources"`
	}
	err := c.call(ctx, "resources/list", nil, &result)
	if err != nil {
		return nil, err
	}
	return result.Resources, nil
}

// ReadResource reads a specific resource by URI from the MCP server.
func (c *Client) ReadResource(ctx context.Context, uri string) (string, error) {
	params := map[string]any{
		"uri": uri,
	}
	var result struct {
		Contents []struct {
			URI      string `json:"uri"`
			MimeType string `json:"mimeType,omitempty"`
			Text     string `json:"text,omitempty"`
		} `json:"contents"`
	}
	err := c.call(ctx, "resources/read", params, &result)
	if err != nil {
		return "", err
	}
	if len(result.Contents) == 0 {
		return "", nil
	}
	return result.Contents[0].Text, nil
}
