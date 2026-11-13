package protocol

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// canonicalSchema mirrors the parts of protocol/schema.json this test checks.
type canonicalSchema struct {
	Version    int      `json:"version"`
	Severities []string `json:"severities"`
	RateKinds  []string `json:"rate_kinds"`
	EventTypes []string `json:"event_types"`
	Messages   map[string]struct {
		Direction string   `json:"direction"`
		Fields    []string `json:"fields"`
	} `json:"messages"`
	Definitions map[string]struct {
		Fields []string `json:"fields"`
	} `json:"definitions"`
}

// goStructs maps each canonical message type to its Go struct. Adding a message
// to the schema without wiring it here fails TestSchemaCoverage.
func goStructs() map[string]reflect.Type {
	return map[string]reflect.Type{
		TypeRegister:    reflect.TypeOf(Register{}),
		TypeHeartbeat:   reflect.TypeOf(Heartbeat{}),
		TypePollResult:  reflect.TypeOf(PollResult{}),
		TypeRateEvent:   reflect.TypeOf(RateEvent{}),
		TypeAssign:      reflect.TypeOf(Assign{}),
		TypeAssignDelta: reflect.TypeOf(AssignDelta{}),
		TypeConfig:      reflect.TypeOf(Config{}),
		TypeSnapshot:    reflect.TypeOf(Snapshot{}),
		TypeStateUpdate: reflect.TypeOf(StateUpdate{}),
		TypeEvent:       reflect.TypeOf(Event{}),
		TypeNodeStatus:  reflect.TypeOf(NodeStatus{}),
	}
}

func loadSchema(t *testing.T) canonicalSchema {
	t.Helper()
	path := filepath.Join("..", "..", "protocol", "schema.json")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read schema: %v", err)
	}
	var s canonicalSchema
	if err := json.Unmarshal(b, &s); err != nil {
		t.Fatalf("parse schema: %v", err)
	}
	return s
}

// jsonFields returns the json tag names of a struct's exported fields, in
// declaration order, dropping ",omitempty"-style options.
func jsonFields(rt reflect.Type) []string {
	var out []string
	for i := 0; i < rt.NumField(); i++ {
		tag := rt.Field(i).Tag.Get("json")
		if tag == "" || tag == "-" {
			continue
		}
		out = append(out, strings.Split(tag, ",")[0])
	}
	return out
}

func TestVersionMatchesSchema(t *testing.T) {
	s := loadSchema(t)
	if Version != s.Version {
		t.Fatalf("protocol.Version=%d but schema version=%d", Version, s.Version)
	}
}

func TestSchemaCoverage(t *testing.T) {
	s := loadSchema(t)
	structs := goStructs()
	for name := range s.Messages {
		if _, ok := structs[name]; !ok {
			t.Errorf("schema message %q has no Go struct in goStructs()", name)
		}
	}
	for name := range structs {
		if _, ok := s.Messages[name]; !ok {
			t.Errorf("Go struct %q is not in the schema", name)
		}
	}
}

func TestMessageFieldsMatchSchema(t *testing.T) {
	s := loadSchema(t)
	for name, rt := range goStructs() {
		msg, ok := s.Messages[name]
		if !ok {
			continue // reported by TestSchemaCoverage
		}
		got := jsonFields(rt)
		want := append([]string(nil), msg.Fields...)
		sort.Strings(got)
		sort.Strings(want)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("message %q field mismatch:\n  go:     %v\n  schema: %v", name, got, want)
		}
	}
}

func TestDefinitionFieldsMatchSchema(t *testing.T) {
	s := loadSchema(t)
	defs := map[string]reflect.Type{
		"fingerprint":  reflect.TypeOf(Fingerprint{}),
		"assignment":   reflect.TypeOf(Assignment{}),
		"sku_diff":     reflect.TypeOf(SkuDiff{}),
		"project_meta": reflect.TypeOf(ProjectMeta{}),
		"screen_diff":  reflect.TypeOf(ScreenDiff{}),
	}
	for name, rt := range defs {
		def, ok := s.Definitions[name]
		if !ok {
			t.Errorf("schema definition %q missing", name)
			continue
		}
		got := jsonFields(rt)
		want := append([]string(nil), def.Fields...)
		sort.Strings(got)
		sort.Strings(want)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("definition %q field mismatch:\n  go:     %v\n  schema: %v", name, got, want)
		}
	}
	for name := range s.Definitions {
		if _, ok := defs[name]; !ok {
			t.Errorf("schema definition %q has no Go type check", name)
		}
	}
}

// TestEnumsMatchSchema guards the const enumerations against the schema lists.
func TestEnumsMatchSchema(t *testing.T) {
	s := loadSchema(t)
	check := func(label string, got, want []string) {
		gc := append([]string(nil), got...)
		wc := append([]string(nil), want...)
		sort.Strings(gc)
		sort.Strings(wc)
		if !reflect.DeepEqual(gc, wc) {
			t.Errorf("%s mismatch:\n  go:     %v\n  schema: %v", label, gc, wc)
		}
	}
	check("severities", []string{
		string(SeverityCritical), string(SeverityNotable),
		string(SeverityInfo), string(SeverityOps),
	}, s.Severities)
	check("rate_kinds", []string{
		string(RateSoftLimited), string(RateHardBlocked),
	}, s.RateKinds)
	check("event_types", allEventTypes(), s.EventTypes)
}

func allEventTypes() []string {
	return []string{
		string(EventSkuRestock), string(EventSkuOnsale), string(EventSkuAdded),
		string(EventSkuSoldout), string(EventSkuPriceChange), string(EventSkuThresholdChange),
		string(EventSkuLowStock), string(EventSkuRemoved), string(EventScreenOnsale), string(EventScreenSoldout),
		string(EventScreenAdded), string(EventScreenRemoved), string(EventDaySoldout),
		string(EventProjectHotOn), string(EventProjectHotOff), string(EventProjectOnsale),
		string(EventProjectPresale), string(EventProjectSoldout), string(EventProjectWindowOpen),
		string(EventProjectPriceBandChange), string(EventProjectStatusChange), string(EventWishMilestone),
		string(EventRisk412), string(EventRisk429), string(EventNodeOnline),
		string(EventNodeOffline), string(EventMonitorAdded), string(EventMonitorRemoved),
	}
}
