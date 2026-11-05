package media

import (
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert/kitutil"
	"github.com/QuantumNous/new-api/relaykit/types"
)

// UnsupportedContent rejects media loss independently of the opt-in tool-loss policy.
func UnsupportedContent(kind string, target types.RelayFormat) error {
	return &types.ConversionLossError{Diagnostics: []types.ConversionDiagnostic{{
		Code: "unsupported_content", Severity: types.ConversionDiagnosticError, To: target,
		Message: fmt.Sprintf("%s content cannot be converted to %s without losing content; use a compatible native endpoint", kind, target),
	}}}
}

// ValidateClaudeVideo prevents unsupported video blocks, including nested tool
// results, from disappearing during Claude request conversion.
func ValidateClaudeVideo(blocks []dto.ClaudeMediaMessage, target types.RelayFormat) error {
	for _, block := range blocks {
		switch block.Type {
		case "video", "input_video", "video_url":
			return UnsupportedContent("video", target)
		}
		if block.Source != nil && strings.HasPrefix(strings.ToLower(strings.TrimSpace(block.Source.MediaType)), "video/") {
			return UnsupportedContent("video", target)
		}
		if block.Type == "tool_result" && block.Content != nil {
			if _, text := block.Content.(string); text {
				continue
			}
			children, err := kitutil.Any2Type[[]dto.ClaudeMediaMessage](block.Content)
			if err != nil {
				return err
			}
			if err := ValidateClaudeVideo(children, target); err != nil {
				return err
			}
		}
	}
	return nil
}
