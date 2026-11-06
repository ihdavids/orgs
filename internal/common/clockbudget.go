package common

// Wire types for time budgets (features.clockBudget, org-clock-budget):
// GET /budgets, GET /budgets/clocked, POST /budgets/set. The server half is
// internal/app/orgs/clockbudget.go.

// BudgetPeriods are the periods a budget can be given for, shortest first:
// the property is BUDGET_ and the name in capitals (BUDGET_WEEK).
var BudgetPeriods = []string{"day", "week", "month", "quarter", "year"}

// BudgetItem is one heading's budget for one period.
type BudgetItem struct {
	Hash     string
	Headline string
	Filename string
	Line     int
	Olp      []string `json:",omitempty"` // the headings above it, outermost first
	Status   string   `json:",omitempty"`
	Tags     []string `json:",omitempty"`

	Period     string  // day, week, month, quarter or year
	Property   string  // BUDGET_WEEK
	Value      string  // the property as written
	Budget     float64 // minutes
	Used       float64 // minutes clocked in the period, the subtree's, a running clock to now
	Remaining  float64 // Budget - Used, negative when over
	Percent    float64 // Used / Budget * 100
	Pace       float64 // how far through the period now is, 0..1
	Expected   float64 // Budget * Pace: what would be on track by now
	Over       bool    // Used > Budget
	Ahead      bool    // Used > Expected: spending faster than the period is passing
	Running    bool    // a clock is running in the subtree
	PeriodFrom string  // YYYY-MM-DD, inclusive
	PeriodTo   string  // YYYY-MM-DD, exclusive
}

// BudgetPeriod sums one period, org-clock-budget's two summary rows.
type BudgetPeriod struct {
	Name       string
	Label      string // "week 2026-W45", "November 2026"
	From, To   string // YYYY-MM-DD; To exclusive
	Pace       float64
	Count      int
	Budgeted   float64 // minutes given out in budgets
	Clocked    float64 // minutes clocked against them
	Budgetable float64 // dailyHours * days in the period, minutes
}

// BudgetReport is GET /budgets.
type BudgetReport struct {
	Ok         bool
	Off        bool   `json:",omitempty"`
	Msg        string `json:",omitempty"`
	Date       string // the day the periods are around
	DailyHours float64
	Periods    []BudgetPeriod
	Items      []BudgetItem
}

// BudgetClocked is GET /budgets/clocked: the running clock's budgets, its
// own heading's and every budgeted heading above it.
type BudgetClocked struct {
	Ok       bool
	Off      bool   `json:",omitempty"`
	Msg      string `json:",omitempty"`
	Active   bool   // a clock is running
	Hash     string `json:",omitempty"`
	Headline string `json:",omitempty"`
	Items    []BudgetItem
}

// BudgetSet is POST /budgets/set. An empty Value takes the budget off.
type BudgetSet struct {
	Hash   string
	Period string
	Value  string
}
