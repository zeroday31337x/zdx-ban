package memory

import (
	"context"
	"fmt"
	"time"
	"zdx-ban/internal/measurement"
)

func ApplyCurrentMeasurement(ctx context.Context, s Store, id string, m measurement.Result) (UpdateEvent, error) {
	r, ok, e := s.Get(ctx, id)
	if e != nil || !ok {
		return UpdateEvent{}, fmt.Errorf("memory record not found")
	}
	ev := UpdateEvent{RecordID: id, PreviousStatus: r.Status, MeasurementID: m.ID, At: time.Now().UTC()}
	switch m.Outcome {
	case measurement.Supported:
		ev.Action = "RETAIN"
		ev.NewStatus = Supported
		ev.Reason = "independent current measurement consistent with memory"
		r.Status = Supported
	case measurement.Contradicted:
		if m.Authoritative() {
			ev.Action = "SUPERSEDE"
			ev.NewStatus = Contradicted
			ev.Reason = "authoritative current measurement overrides remembered guidance"
			r.Status = Contradicted
		} else {
			ev.Action = "DOWNGRADE"
			ev.NewStatus = Uncertain
			ev.Reason = "non-authoritative contradiction"
			r.Status = Uncertain
		}
	case measurement.Inconclusive, measurement.NotMeasured:
		ev.Action = "PRESERVE"
		ev.NewStatus = r.Status
		ev.Reason = "insufficient current evidence"
	case measurement.Error:
		ev.Action = "PRESERVE"
		ev.NewStatus = r.Status
		ev.Reason = "measurement error is not memory contradiction"
	}
	r.UpdatedAt = ev.At
	r.RelatedMeasurementIDs = append(r.RelatedMeasurementIDs, m.ID)
	if r.Metadata == nil {
		r.Metadata = map[string]any{}
	}
	events, _ := r.Metadata["updateEvents"].([]any)
	r.Metadata["updateEvents"] = append(events, map[string]any{"action": ev.Action, "reason": ev.Reason, "measurementId": m.ID, "at": ev.At})
	if e = s.Replace(ctx, r); e != nil {
		return UpdateEvent{}, e
	}
	return ev, nil
}
func Supersede(ctx context.Context, s Store, oldID string, replacement Record, reason string) (UpdateEvent, error) {
	old, ok, e := s.Get(ctx, oldID)
	if e != nil || !ok {
		return UpdateEvent{}, fmt.Errorf("memory not found")
	}
	if e = s.Append(ctx, replacement); e != nil {
		return UpdateEvent{}, e
	}
	old.Status = Superseded
	old.SupersededBy = replacement.ID
	old.UpdatedAt = time.Now().UTC()
	if e = s.Replace(ctx, old); e != nil {
		return UpdateEvent{}, e
	}
	return UpdateEvent{RecordID: oldID, Action: "SUPERSEDE", Reason: reason, PreviousStatus: Active, NewStatus: Superseded, At: old.UpdatedAt}, nil
}
