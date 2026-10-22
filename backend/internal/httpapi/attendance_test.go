package httpapi

import (
	"math"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestCalculateLateMinutesUsesJakartaFullMinutes(t *testing.T) {
	date, err := parseEffectiveDate("2025-06-02")
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name          string
		clock         string
		wantMinutes   int
		wantLateEvent bool
	}{
		{name: "on time", clock: "08:30", wantMinutes: 0},
		{name: "partial minute", clock: "08:30:59", wantMinutes: 0, wantLateEvent: true},
		{name: "one full minute", clock: "08:31", wantMinutes: 1, wantLateEvent: true},
		{name: "thirty three minutes", clock: "09:03:45", wantMinutes: 33, wantLateEvent: true},
		{name: "early arrival", clock: "08:15", wantMinutes: 0},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			parsed, err := time.Parse("15:04:05", test.clock)
			if err != nil {
				parsed, err = time.Parse("15:04", test.clock)
			}
			if err != nil {
				t.Fatal(err)
			}
			at := atLocalTime(date, parsed)
			minutes, late, err := calculateLateMinutes(at, date, "08:30:00")
			if err != nil {
				t.Fatal(err)
			}
			if minutes != test.wantMinutes || late != test.wantLateEvent {
				t.Fatalf("calculateLateMinutes() = (%d, %v), want (%d, %v)", minutes, late, test.wantMinutes, test.wantLateEvent)
			}
		})
	}
}

func TestIsWorkingDayUsesMondayThroughFriday(t *testing.T) {
	for _, test := range []struct {
		date string
		want bool
	}{
		{date: "2025-06-02", want: true},
		{date: "2025-06-06", want: true},
		{date: "2025-06-07", want: false},
		{date: "2025-06-08", want: false},
	} {
		date, err := parseEffectiveDate(test.date)
		if err != nil {
			t.Fatal(err)
		}
		if got := isWorkingDay(date, []int16{1, 2, 3, 4, 5}); got != test.want {
			t.Errorf("isWorkingDay(%s) = %v, want %v", test.date, got, test.want)
		}
	}
}

func TestCalendarEntryOverridesScheduledWorkday(t *testing.T) {
	tests := []struct {
		name     string
		date     string
		kind     string
		hasEntry bool
		wantWork bool
	}{
		{name: "weekday without override", date: "2025-06-02", wantWork: true},
		{name: "public holiday closes weekday", date: "2025-06-02", kind: "public_holiday", hasEntry: true},
		{name: "collective leave closes weekday", date: "2025-06-02", kind: "collective_leave", hasEntry: true},
		{name: "special workday opens Saturday", date: "2025-06-07", kind: "company_workday", hasEntry: true, wantWork: true},
		{name: "Saturday remains closed without override", date: "2025-06-07"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			date, err := parseEffectiveDate(test.date)
			if err != nil {
				t.Fatal(err)
			}
			if got := calendarWorkday(date, test.kind, test.hasEntry, []int16{1, 2, 3, 4, 5}); got != test.wantWork {
				t.Fatalf("calendarWorkday() = %v, want %v", got, test.wantWork)
			}
		})
	}
}

func TestHaversineMeters(t *testing.T) {
	if got := haversineMeters(-6.2, 106.8, -6.2, 106.8); got != 0 {
		t.Fatalf("same point distance = %f, want 0", got)
	}
	if got := haversineMeters(0, 0, 0, 0.001); got < 111 || got > 112 {
		t.Fatalf("one millidegree longitude distance = %f, want about 111.2m", got)
	}
	if got := haversineMeters(0, 0, 0, 180); math.IsNaN(got) || got < 20000000 {
		t.Fatalf("antipodal distance = %f, want a finite distance near half the earth's circumference", got)
	}
}

func TestValidateLocation(t *testing.T) {
	geofence := officeGeofence{
		ID: uuid.New(), Latitude: -6.2, Longitude: 106.8, RadiusMeters: 50,
	}
	tests := []struct {
		name       string
		input      locationInput
		wantStatus int
	}{
		{name: "inside", input: locationInput{Latitude: -6.2, Longitude: 106.8, Accuracy: 30}, wantStatus: http.StatusOK},
		{name: "accuracy too low", input: locationInput{Latitude: -6.2, Longitude: 106.8, Accuracy: 31}, wantStatus: http.StatusUnprocessableEntity},
		{name: "outside fence", input: locationInput{Latitude: -6.2, Longitude: 106.801, Accuracy: 10}, wantStatus: http.StatusForbidden},
		{name: "invalid latitude", input: locationInput{Latitude: 91, Longitude: 106.8, Accuracy: 10}, wantStatus: http.StatusBadRequest},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, status, _ := validateLocation(test.input, geofence)
			if status != test.wantStatus {
				t.Fatalf("validateLocation() status = %d, want %d", status, test.wantStatus)
			}
		})
	}
}

func TestParseClockSupportsDatabaseTimeAndRejectsInvalidValues(t *testing.T) {
	for _, value := range []string{"08:30", "18:00:00"} {
		if _, ok := parseClock(value); !ok {
			t.Errorf("parseClock(%q) rejected a valid clock value", value)
		}
	}
	for _, value := range []string{"25:00", "8:30", ""} {
		if _, ok := parseClock(value); ok {
			t.Errorf("parseClock(%q) accepted an invalid clock value", value)
		}
	}
}
