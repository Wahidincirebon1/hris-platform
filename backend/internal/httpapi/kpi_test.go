package httpapi

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func TestValidKPICycleTransitions(t *testing.T) {
	tests := []struct {
		oldStatus string
		newStatus string
		want      bool
	}{
		{oldStatus: "planning", newStatus: "planning", want: true},
		{oldStatus: "planning", newStatus: "open", want: true},
		{oldStatus: "open", newStatus: "closed", want: true},
		{oldStatus: "closed", newStatus: "open"},
		{oldStatus: "open", newStatus: "planning"},
		{oldStatus: "closed", newStatus: "planning"},
	}
	for _, test := range tests {
		if got := validKPICycleTransition(test.oldStatus, test.newStatus); got != test.want {
			t.Errorf("validKPICycleTransition(%q, %q) = %v, want %v", test.oldStatus, test.newStatus, got, test.want)
		}
	}
}

func TestWeightedKPIScoreRequiresCompleteHundredPercentWeights(t *testing.T) {
	tests := []struct {
		name    string
		weights []float64
		scores  []float64
		want    float64
		valid   bool
	}{
		{name: "weighted average", weights: []float64{25, 75}, scores: []float64{80, 90}, want: 87.5, valid: true},
		{name: "score may be zero", weights: []float64{100}, scores: []float64{0}, valid: true},
		{name: "score may be one hundred", weights: []float64{100}, scores: []float64{100}, want: 100, valid: true},
		{name: "weights must total one hundred", weights: []float64{25, 70}, scores: []float64{80, 90}},
		{name: "score outside scale", weights: []float64{100}, scores: []float64{101}},
		{name: "missing score", weights: []float64{50, 50}, scores: []float64{80}},
		{name: "invalid weight", weights: []float64{0, 100}, scores: []float64{80, 90}},
		{name: "non finite score", weights: []float64{100}, scores: []float64{math.NaN()}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, valid := weightedKPIScore(test.weights, test.scores)
			if valid != test.valid {
				t.Fatalf("weightedKPIScore() valid = %v, want %v", valid, test.valid)
			}
			if valid && math.Abs(got-test.want) > 1e-9 {
				t.Fatalf("weightedKPIScore() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestSafeCSVCellPreventsSpreadsheetFormulaInjection(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{input: "=1+1", want: "'=1+1"},
		{input: "+cmd", want: "'+cmd"},
		{input: "@SUM(A1)", want: "'@SUM(A1)"},
		{input: "-1.25", want: "-1.25"},
		{input: "-cmd", want: "'-cmd"},
		{input: "Normal text", want: "Normal text"},
	}
	for _, test := range tests {
		if got := safeCSVCell(test.input); got != test.want {
			t.Errorf("safeCSVCell(%q) = %q, want %q", test.input, got, test.want)
		}
	}
}

func TestKPIReportScopeRestrictsRoleAndBranch(t *testing.T) {
	managerID := uuid.New()
	branchID := uuid.New()
	otherBranchID := uuid.New()
	tests := []struct {
		name      string
		user      principal
		rawBranch string
		permitted bool
		where     string
		argCount  int
	}{
		{
			name:  "HR can query all branches",
			user:  principal{Role: "hr"},
			where: "WHERE s.kpi_year = $1", argCount: 1, permitted: true,
		},
		{
			name:      "HR can select branch",
			user:      principal{Role: "superadmin"},
			rawBranch: branchID.String(), where: "WHERE s.kpi_year = $1 AND s.branch_id = $2",
			argCount: 2, permitted: true,
		},
		{
			name:      "invalid HR branch rejected",
			user:      principal{Role: "hr"},
			rawBranch: "not-a-uuid", permitted: false,
		},
		{
			name:  "manager restricted to direct reports",
			user:  principal{ID: managerID, BranchID: branchID, Role: "manager"},
			where: "WHERE s.kpi_year = $1 AND s.manager_id = $2", argCount: 2, permitted: true,
		},
		{
			name:      "manager cannot select another branch",
			user:      principal{ID: managerID, BranchID: branchID, Role: "manager"},
			rawBranch: otherBranchID.String(), permitted: false,
		},
		{
			name: "employee cannot query report",
			user: principal{Role: "employee"}, permitted: false,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			where, args, permitted := kpiReportScope(test.user, 2026, test.rawBranch)
			if permitted != test.permitted {
				t.Fatalf("permitted = %v, want %v", permitted, test.permitted)
			}
			if !permitted {
				return
			}
			if where != test.where {
				t.Errorf("where = %q, want %q", where, test.where)
			}
			if len(args) != test.argCount || args[0] != 2026 {
				t.Errorf("args = %#v, want %d arguments starting with year 2026", args, test.argCount)
			}
			if test.user.Role == "manager" && (len(args) != 2 || args[1] != managerID) {
				t.Errorf("manager scope args = %#v, want manager ID %s", args, managerID)
			}
		})
	}
}

type testCalendarQuery struct {
	kinds map[string]string
}

func (q testCalendarQuery) QueryRow(_ context.Context, _ string, args ...any) pgx.Row {
	date := args[0].(time.Time).Format("2006-01-02")
	kind, found := q.kinds[date]
	if !found {
		return testCalendarRow{err: pgx.ErrNoRows}
	}
	return testCalendarRow{kind: kind}
}

type testCalendarRow struct {
	kind string
	err  error
}

func (r testCalendarRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	*dest[0].(*string) = r.kind
	return nil
}

func TestKPIAppealDeadlineSkipsWeekendsAndCalendarHolidays(t *testing.T) {
	finalizedAt := time.Date(2026, time.August, 7, 15, 30, 0, 0, jakartaTime)
	calendar := testCalendarQuery{kinds: map[string]string{
		"2026-08-10": "public_holiday",
	}}
	got, err := kpiAppealDeadline(context.Background(), calendar, finalizedAt, 5)
	if err != nil {
		t.Fatalf("kpiAppealDeadline() error = %v", err)
	}
	want := time.Date(2026, time.August, 17, 0, 0, 0, 0, jakartaTime)
	if !got.Equal(want) {
		t.Errorf("kpiAppealDeadline() = %s, want %s", got.Format("2006-01-02"), want.Format("2006-01-02"))
	}
}
