package assign

import (
	"testing"
	"time"

	"github.com/WittF/fnmdp/shared/protocol"
)

func TestStoreRoundTripsAssignment(t *testing.T) {
	store := NewStore()
	store.Replace([]protocol.Assignment{{
		ProjectID:  100,
		Phase:      1,
		PhaseMod:   10,
		IntervalMs: 30000,
	}})
	snap := store.Snapshot()
	if len(snap) != 1 || snap[0].ProjectID != 100 || snap[0].Phase != 1 || snap[0].Interval != 30*time.Second {
		t.Fatalf("snapshot = %+v", snap)
	}
	wire := ToProtocol(Assignment{ProjectID: 100, Interval: 30 * time.Second})
	if wire.ProjectID != 100 || wire.IntervalMs != 30000 || wire.PhaseMod != DefaultPhaseMod {
		t.Fatalf("protocol assignment = %+v", wire)
	}
}
