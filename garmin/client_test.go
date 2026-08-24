package garmin

import (
	"encoding/json"
	"testing"
)

func TestFindUnitIDForDevice(t *testing.T) {
	tests := []struct {
		name        string
		devices     map[string]any
		wantUnitID  uint32
		shouldMatch bool
	}{
		{
			name: "FIT product ID",
			devices: map[string]any{"devices": []any{
				map[string]any{"productId": json.Number("3992"), "unitId": json.Number("111")},
				map[string]any{"productId": json.Number("4257"), "unitId": json.Number("987654")},
			}},
			wantUnitID:  987654,
			shouldMatch: true,
		},
		{
			name: "model name fallback when device-service ID differs",
			devices: map[string]any{"devices": []any{
				map[string]any{"productId": json.Number("9999"), "productDisplayName": "Forerunner® 265", "unitId": json.Number("987654")},
			}},
			wantUnitID:  987654,
			shouldMatch: true,
		},
		{
			name: "does not match the 265S by partial name",
			devices: map[string]any{"devices": []any{
				map[string]any{"productId": json.Number("9999"), "productDisplayName": "Forerunner 265S", "unitId": json.Number("987654")},
			}},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			unitID, ok := findUnitIDForDevice(test.devices, 4257, "Garmin Forerunner 265")
			if ok != test.shouldMatch {
				t.Fatalf("match = %t, want %t", ok, test.shouldMatch)
			}
			if unitID != test.wantUnitID {
				t.Errorf("unit ID = %d, want %d", unitID, test.wantUnitID)
			}
		})
	}
}
