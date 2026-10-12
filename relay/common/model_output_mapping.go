package common

import (
	"bytes"
	"strings"

	hostcommon "github.com/QuantumNous/new-api/common"
)

// ModelOutputMapper rewrites protocol model-name fields according to a
// channel's response model mapping. Mapping keys are matched case-insensitively.
type ModelOutputMapper struct {
	names map[string]string
}

const RequestedModelOutputValue = "$requested_model"

func NewModelOutputMapper(raw string) (*ModelOutputMapper, error) {
	configured := make(map[string]string)
	if strings.TrimSpace(raw) == "" {
		raw = "{}"
	}
	if err := hostcommon.UnmarshalJsonStr(raw, &configured); err != nil {
		return nil, err
	}
	names := make(map[string]string, len(configured))
	for upstream, public := range configured {
		upstream = strings.ToLower(strings.TrimSpace(upstream))
		public = strings.TrimSpace(public)
		if upstream != "" && public != "" {
			names[upstream] = public
		}
	}
	return &ModelOutputMapper{names: names}, nil
}

func (info *RelayInfo) ResponseModelOutputMapper() (*ModelOutputMapper, error) {
	if info == nil || info.ChannelMeta == nil {
		return nil, nil
	}
	raw := info.ChannelSetting.ModelOutputMapping
	useRequestModel := info.ChannelSetting.ResponseModelUseRequestModel
	if info.modelOutputMappingParsed && info.modelOutputMappingRaw == raw && info.modelOutputUseRequest == useRequestModel {
		return info.modelOutputMapper, info.modelOutputMappingErr
	}
	info.modelOutputMappingRaw = raw
	info.modelOutputUseRequest = useRequestModel
	info.modelOutputMappingParsed = true
	info.modelOutputMapper = nil
	info.modelOutputMappingErr = nil
	if (raw == "" || raw == "{}") && !useRequestModel {
		return nil, nil
	}
	info.modelOutputMapper, info.modelOutputMappingErr = NewModelOutputMapper(raw)
	return info.modelOutputMapper, info.modelOutputMappingErr
}

// RewritePayload handles both plain JSON responses and complete SSE data
// lines. Unrecognized or partial payloads pass through unchanged.
func (m *ModelOutputMapper) RewritePayload(data []byte) []byte {
	return m.rewritePayload(data, "", "", false)
}

// RewritePayloadForRequest restores the client-requested model when the
// provider returned the actual upstream model used for this request. Static
// output mappings remain the fallback for other returned model names.
func (m *ModelOutputMapper) RewritePayloadForRequest(data []byte, upstreamModel, requestModel string, restoreRequestModel bool) []byte {
	upstreamModel = strings.TrimSpace(upstreamModel)
	requestModel = strings.TrimSpace(requestModel)
	if restoreRequestModel && strings.EqualFold(upstreamModel, requestModel) {
		return m.rewritePayload(data, "", "", false)
	}
	return m.rewritePayload(data, upstreamModel, requestModel, restoreRequestModel)
}

func (m *ModelOutputMapper) rewritePayload(data []byte, upstreamModel, requestModel string, restoreRequestModel bool) []byte {
	if m == nil || (len(m.names) == 0 && (upstreamModel == "" || requestModel == "")) || len(data) == 0 {
		return data
	}
	if rewritten, changed := m.rewriteJSON(data, upstreamModel, requestModel, restoreRequestModel); changed {
		return rewritten
	}
	if !bytes.Contains(data, []byte("data:")) {
		return data
	}

	lines := bytes.SplitAfter(data, []byte("\n"))
	changed := false
	for i, line := range lines {
		lineEnd := []byte(nil)
		content := line
		if bytes.HasSuffix(content, []byte("\n")) {
			lineEnd = []byte("\n")
			content = content[:len(content)-1]
		}
		if bytes.HasSuffix(content, []byte("\r")) {
			lineEnd = append([]byte("\r"), lineEnd...)
			content = content[:len(content)-1]
		}
		if !bytes.HasPrefix(content, []byte("data:")) {
			continue
		}
		prefixLength := len("data:")
		if len(content) > prefixLength && content[prefixLength] == ' ' {
			prefixLength++
		}
		rewritten, lineChanged := m.rewriteJSON(content[prefixLength:], upstreamModel, requestModel, restoreRequestModel)
		if !lineChanged {
			continue
		}
		lines[i] = append(append(append([]byte(nil), content[:prefixLength]...), rewritten...), lineEnd...)
		changed = true
	}
	if !changed {
		return data
	}
	return bytes.Join(lines, nil)
}

func (m *ModelOutputMapper) rewriteJSON(data []byte, upstreamModel, requestModel string, restoreRequestModel bool) ([]byte, bool) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return data, false
	}
	switch trimmed[0] {
	case '{':
		object := make(map[string]hostcommon.RawMessage)
		if err := hostcommon.Unmarshal(trimmed, &object); err != nil || !m.rewriteObject(object, upstreamModel, requestModel, restoreRequestModel) {
			return data, false
		}
		rewritten, err := hostcommon.Marshal(object)
		return rewrittenOrOriginal(data, rewritten, err)
	case '[':
		var items []hostcommon.RawMessage
		if err := hostcommon.Unmarshal(trimmed, &items); err != nil || !m.rewriteArray(items, upstreamModel, requestModel, restoreRequestModel) {
			return data, false
		}
		rewritten, err := hostcommon.Marshal(items)
		return rewrittenOrOriginal(data, rewritten, err)
	default:
		return data, false
	}
}

func rewrittenOrOriginal(original, rewritten []byte, err error) ([]byte, bool) {
	if err != nil {
		return original, false
	}
	return rewritten, true
}

func (m *ModelOutputMapper) rewriteObject(object map[string]hostcommon.RawMessage, upstreamModel, requestModel string, restoreRequestModel bool) bool {
	changed := false
	for key, child := range object {
		if key == "model" || key == "modelVersion" || key == "model_version" {
			var modelName string
			if err := hostcommon.Unmarshal(child, &modelName); err == nil {
				mapped := ""
				exists := false
				if restoreRequestModel && upstreamModel != "" && requestModel != "" && strings.EqualFold(strings.TrimSpace(modelName), upstreamModel) {
					mapped = requestModel
					exists = true
				} else {
					mapped, exists = m.names[strings.ToLower(modelName)]
					if exists && mapped == RequestedModelOutputValue {
						mapped = requestModel
						exists = requestModel != ""
					}
				}
				if exists {
					encoded, err := hostcommon.Marshal(mapped)
					if err == nil {
						object[key] = encoded
						changed = true
					}
				}
			}
		}
		if rewritten, childChanged := m.rewriteJSON(child, upstreamModel, requestModel, restoreRequestModel); childChanged {
			object[key] = rewritten
			changed = true
		}
	}
	return changed
}

func (m *ModelOutputMapper) rewriteArray(items []hostcommon.RawMessage, upstreamModel, requestModel string, restoreRequestModel bool) bool {
	changed := false
	for i, item := range items {
		if rewritten, itemChanged := m.rewriteJSON(item, upstreamModel, requestModel, restoreRequestModel); itemChanged {
			items[i] = rewritten
			changed = true
		}
	}
	return changed
}
