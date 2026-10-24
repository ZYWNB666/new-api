package moonshot

import (
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestConvertOpenAIRequestPreservesPromptCacheOptions(t *testing.T) {
	for _, model := range []string{"kimi-k3", "kimi-k3.1-pro"} {
		for _, stream := range []bool{false, true} {
			for _, options := range []string{`{"mode":"implicit","ttl":"1h"}`, `{"mode":"implicit","ttl":"5m"}`, ""} {
				t.Run(fmt.Sprintf("%s/stream=%t/options=%s", model, stream, options), func(t *testing.T) {
					raw := fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"hello"}],"stream":%t`, model, stream)
					if options != "" {
						raw += `,"prompt_cache_options":` + options
					}
					raw += "}"
					var request dto.GeneralOpenAIRequest
					require.NoError(t, common.UnmarshalJsonStr(raw, &request))
					converted, err := (&Adaptor{}).ConvertOpenAIRequest(nil, nil, &request)
					require.NoError(t, err)
					encoded, err := common.Marshal(converted)
					require.NoError(t, err)
					if options == "" {
						assert.False(t, gjson.GetBytes(encoded, "prompt_cache_options").Exists())
					} else {
						assert.JSONEq(t, options, gjson.GetBytes(encoded, "prompt_cache_options").Raw)
					}
				})
			}
		}
	}
}

func TestConvertOpenAIRequestKimiK26UsesOnlyAllowedTemperature(t *testing.T) {
	request := &dto.GeneralOpenAIRequest{
		Model:       "kimi-k2.6",
		Temperature: common.GetPointer[float64](0.7),
	}
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "kimi-k2.6",
		},
	}

	converted, err := (&Adaptor{}).ConvertOpenAIRequest(nil, info, request)

	require.NoError(t, err)
	convertedRequest, ok := converted.(*dto.GeneralOpenAIRequest)
	require.True(t, ok)
	require.NotNil(t, convertedRequest.Temperature)
	require.Equal(t, 1.0, *convertedRequest.Temperature)
}

func TestConvertOpenAIRequestKimiK26KeepsOmittedTemperatureOmitted(t *testing.T) {
	request := &dto.GeneralOpenAIRequest{
		Model: "kimi-k2.6",
	}
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "kimi-k2.6",
		},
	}

	converted, err := (&Adaptor{}).ConvertOpenAIRequest(nil, info, request)

	require.NoError(t, err)
	convertedRequest, ok := converted.(*dto.GeneralOpenAIRequest)
	require.True(t, ok)
	require.Nil(t, convertedRequest.Temperature)
}

func TestConvertOpenAIRequestOtherMoonshotModelKeepsTemperature(t *testing.T) {
	request := &dto.GeneralOpenAIRequest{
		Model:       "kimi-k2.5",
		Temperature: common.GetPointer[float64](0.7),
	}
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "kimi-k2.5",
		},
	}

	converted, err := (&Adaptor{}).ConvertOpenAIRequest(nil, info, request)

	require.NoError(t, err)
	convertedRequest, ok := converted.(*dto.GeneralOpenAIRequest)
	require.True(t, ok)
	require.NotNil(t, convertedRequest.Temperature)
	require.Equal(t, 0.7, *convertedRequest.Temperature)
}
