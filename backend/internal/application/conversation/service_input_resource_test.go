package conversation

import (
	"errors"
	"testing"
)

func TestSelectGatewayInputResourcesRequiresCurrentUniqueRefs(t *testing.T) {
	available := []GatewayInputResource{
		{ResourceRef: "skill_current", Kind: "skill", Name: "review"},
		{ResourceRef: "app_current", Kind: "app-mention", Name: "calendar"},
	}
	selected, err := selectGatewayInputResources([]string{"skill_current", "app_current"}, available)
	if err != nil || len(selected) != 2 || selected[0].Kind != "skill" || selected[1].Kind != "app-mention" {
		t.Fatalf("valid input resources rejected: %#v %v", selected, err)
	}
	for _, refs := range [][]string{{"skill_old"}, {"skill_current", "skill_current"}} {
		if _, err = selectGatewayInputResources(refs, available); !errors.Is(err, ErrInvalidExecutionTarget) {
			t.Fatalf("invalid resource refs accepted: %#v %v", refs, err)
		}
	}
}
