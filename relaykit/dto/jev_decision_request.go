package dto

import (
	"net/http"

	"github.com/QuantumNous/new-api/relaykit/types"
)

// JevDecisionRequest is the Jev decision request (OpenRouter /api/alpha/decisions
// or TypeSafe /v1/systemone). RawBody preserves the original JSON so unknown
// fields (state, questions) are forwarded intact.
type JevDecisionRequest struct {
	Model   string `json:"model"`
	RawBody []byte `json:"-"`
}

func (r *JevDecisionRequest) GetTokenCountMeta() *types.TokenCountMeta {
	combineText := ""
	if len(r.RawBody) > 0 {
		combineText = string(r.RawBody)
	}
	return &types.TokenCountMeta{
		CombineText: combineText,
		TokenType:   types.TokenTypeTokenizer,
	}
}

func (r *JevDecisionRequest) IsStream(_ *http.Request) bool {
	return false
}

func (r *JevDecisionRequest) SetModelName(modelName string) {
	if modelName != "" {
		r.Model = modelName
	}
}

// JevDecisionUsage mirrors the usage block returned by the decision endpoint.
type JevDecisionUsage struct {
	InputTokens  int     `json:"input_tokens"`
	OutputTokens int     `json:"output_tokens"`
	Cost         float64 `json:"cost"`
}

// JevDecisionResponse is the upstream decision payload. Answers are kept as a
// raw map so every question type (choice/score/noul) is forwarded unchanged.
type JevDecisionResponse struct {
	Model    string                    `json:"model"`
	Answers  map[string]map[string]any `json:"answers"`
	Usage    *JevDecisionUsage         `json:"usage,omitempty"`
	Id       string                    `json:"id,omitempty"`
	Provider string                    `json:"provider,omitempty"`
}
