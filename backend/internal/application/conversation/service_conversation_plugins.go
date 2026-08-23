package conversation

import (
	"fmt"

	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/shared/nativetool"
)

func withConversationPluginTools(
	options map[string]interface{},
	refs []string,
	protocol string,
	enabledKeysJSON string,
) (map[string]interface{}, error) {
	keys, err := nativetool.ResolveConversationPluginRefs(refs, enabledKeysJSON)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrConversationPluginUnavailable, err)
	}
	if len(keys) == 0 {
		return options, nil
	}

	protocolKey := modelOptionPolicyProtocolKey(protocol)
	tools := make([]map[string]interface{}, 0, len(keys))
	for _, key := range keys {
		_, payload, ok := nativetool.ConversationPluginTool(protocolKey, key)
		if !ok {
			return nil, fmt.Errorf("%w: %s does not support %s", ErrConversationPluginUnsupported, protocolKey, key)
		}
		tools = append(tools, payload)
	}

	compiled := cloneModelOptionMap(options)
	if compiled == nil {
		compiled = make(map[string]interface{}, 1)
	}
	compiled["tools"] = tools
	return compiled, nil
}
