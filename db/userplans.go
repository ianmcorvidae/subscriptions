package db

import (
	"context"
	"time"

	t "github.com/cyverse-de/subscriptions/db/tables"
	"github.com/doug-martin/goqu/v9"
)

// GetActiveUserPlan returns the active user plan for the username passed in.
// Accepts a variable number of QueryOptions, but only WithTX is currently
// supported.
func (d *Database) GetActiveUserPlan(ctx context.Context, username string, opts ...QueryOption) (*UserPlan, error) {
	var (
		err    error
		result UserPlan
		db     GoquDatabase
	)

	_, db = d.querySettings(opts...)

	effStartDate := goqu.I("user_plans.effective_start_date")
	effEndDate := goqu.I("user_plans.effective_end_date")
	currTS := goqu.L("CURRENT_TIMESTAMP")

	query := db.From(t.UserPlans).
		Select(
			t.UserPlans.Col("id").As("id"),
			t.UserPlans.Col("effective_start_date").As("effective_start_date"),
			t.UserPlans.Col("effective_end_date").As("effective_end_date"),
			t.UserPlans.Col("created_by").As("created_by"),
			t.UserPlans.Col("created_at").As("created_at"),
			t.UserPlans.Col("last_modified_by").As("last_modified_by"),
			t.UserPlans.Col("last_modified_at").As("last_modified_at"),

			t.Users.Col("id").As(goqu.C("t.Users.id")),
			t.Users.Col("username").As(goqu.C("t.Users.username")),

			t.Plans.Col("id").As(goqu.C("t.Plans.id")),
			t.Plans.Col("name").As(goqu.C("t.Plans.name")),
			t.Plans.Col("description").As(goqu.C("t.Plans.description")),
		).
		Join(t.Users, goqu.On(t.UserPlans.Col("user_id").Eq(t.Users.Col("id")))).
		Join(t.Plans, goqu.On(t.UserPlans.Col("plan_id").Eq(t.Plans.Col("id")))).
		Where(goqu.And(
			t.Users.Col("username").Eq(username),
			goqu.Or(
				currTS.Between(goqu.Range(effStartDate, effEndDate)),
				goqu.And(currTS.Gt(effStartDate), effEndDate.Is(nil)),
			),
		)).
		Order(effStartDate.Desc()).
		Limit(1).
		Executor()

	if _, err = query.ScanStructContext(ctx, &result); err != nil {
		return nil, err
	}

	log.Debugf("%+v", result)

	return &result, nil
}

func (d *Database) SetActiveUserPlan(ctx context.Context, userID, planID string, opts ...QueryOption) (string, error) {
	_, db := d.querySettings(opts...)

	n := time.Now()
	e := n.AddDate(1, 0, 0)

	query := db.Insert(t.UserPlans).
		Rows(
			goqu.Record{
				"effective_start_date": n,
				"effective_end_date":   e,
				"user_id":              userID,
				"plan_id":              planID,
				"created_by":           "de",
				"last_modified_by":     "de",
			},
		).
		Returning(t.UserPlans.Col("id")).
		Executor()

	var userPlanID string
	if err := query.ScanValsContext(ctx, &userPlanID); err != nil {
		return "", err
	}

	// Add the quota defaults as the t.Quotas for the user plan.
	ds := db.Insert(t.Quotas).
		Cols(
			"resource_type_id",
			"user_plan_id",
			"quota",
			"created_by",
			"last_modified_by",
		).
		FromQuery(
			db.From(t.PQD).
				Select(
					t.PQD.Col("resource_type_id"),
					goqu.V(userPlanID).As("user_plan_id"),
					t.PQD.Col("quota_value").As("quota"),
					goqu.V("de").As("created_by"),
					goqu.V("de").As("last_modified_by"),
				).
				Join(t.Plans, goqu.On(t.PQD.Col("plan_id").Eq(t.Plans.Col("id")))).
				Where(
					t.Plans.Col("id").Eq(planID),
				),
		).
		Executor()

	if _, err := ds.Exec(); err != nil {
		return userPlanID, err
	}

	return userPlanID, nil

}

func (d *Database) UserHasActivePlan(ctx context.Context, username string, opts ...QueryOption) (bool, error) {
	var (
		err error
		db  GoquDatabase
	)

	_, db = d.querySettings(opts...)

	numPlans, err := db.From(t.UserPlans).
		Join(t.Users, goqu.On(t.UserPlans.Col("user_id").Eq(t.Users.Col("id")))).
		Where(t.Users.Col("username").Eq(username)).
		CountContext(ctx)
	if err != nil {
		return false, err
	}

	return numPlans > 0, nil
}

func (d *Database) UserOnPlan(ctx context.Context, username, planName string, opts ...QueryOption) (bool, error) {
	var err error

	_, db := d.querySettings(opts...)

	numPlans, err := db.From(t.UserPlans).
		Join(t.Users, goqu.On(t.UserPlans.Col("user_id").Eq(t.Users.Col("id")))).
		Join(t.Plans, goqu.On(t.UserPlans.Col("plan_id").Eq(t.Plans.Col("id")))).
		Where(
			t.Users.Col("username").Eq(username),
			t.Plans.Col("name").Eq(planName),
		).
		Count()
	if err != nil {
		return false, err
	}

	return numPlans > 0, nil
}

// UserPlanUsages returns a list of Usages associated with a user plan specified
// by the passed in UUID. Accepts a variable number of QueryOptions, though only
// WithTX is currently supported.
func (d *Database) UserPlanUsages(ctx context.Context, userPlanID string, opts ...QueryOption) ([]Usage, error) {
	var (
		err    error
		db     GoquDatabase
		usages []Usage
	)

	_, db = d.querySettings(opts...)

	usagesQuery := db.From(t.Usages).
		Select(
			t.Usages.Col("id").As("id"),
			t.Usages.Col("usage").As("usage"),
			t.Usages.Col("user_plan_id").As("user_plan_id"),
			t.Usages.Col("created_by").As("created_by"),
			t.Usages.Col("created_at").As("created_at"),
			t.Usages.Col("last_modified_by").As("last_modified_by"),
			t.Usages.Col("last_modified_at").As("last_modified_at"),
			t.RT.Col("id").As(goqu.C("resource_types.id")),
			t.RT.Col("name").As(goqu.C("resource_types.name")),
			t.RT.Col("unit").As(goqu.C("resource_types.unit")),
		).
		Join(t.RT, goqu.On(goqu.I("usages.resource_type_id").Eq(goqu.I("resource_types.id")))).
		Where(t.Usages.Col("user_plan_id").Eq(userPlanID)).
		Executor()

	if err = usagesQuery.ScanStructsContext(ctx, &usages); err != nil {
		return nil, err
	}

	return usages, nil
}

// UserPlanQuotas returns a list of t.Quotas associated with the user plan specified
// by the UUID passed in. Accepts a variable number of QueryOptions, though only
// WithTX is currently supported.
func (d *Database) UserPlanQuotas(ctx context.Context, userPlanID string, opts ...QueryOption) ([]Quota, error) {
	var (
		err    error
		db     GoquDatabase
		quotas []Quota
	)

	_, db = d.querySettings(opts...)

	quotasQuery := db.From(t.Quotas).
		Select(
			t.Quotas.Col("id").As("id"),
			t.Quotas.Col("quota").As("quota"),
			t.Quotas.Col("created_by").As("created_by"),
			t.Quotas.Col("created_at").As("created_at"),
			t.Quotas.Col("last_modified_by").As("last_modified_by"),
			t.Quotas.Col("last_modified_at").As("last_modified_at"),
			t.RT.Col("id").As(goqu.C("resource_types.id")),
			t.RT.Col("name").As(goqu.C("resource_types.name")),
			t.RT.Col("unit").As(goqu.C("resource_types.unit")),
		).
		Join(t.RT, goqu.On(goqu.I("t.Quotas.resource_type_id").Eq(goqu.I("resource_types.id")))).
		Where(t.Quotas.Col("user_plan_id").Eq(userPlanID)).
		Executor()

	if err = quotasQuery.ScanStructsContext(ctx, &quotas); err != nil {
		return nil, err
	}

	return quotas, nil
}

// UserPlanQuotaDefaults returns a list of PlanQuotaDefaults associated with the
// plan (not user plan, just plan) specified by the UUID passed in. Accepts a
// variable number of QueryOptions, though only WithTX is currently supported.
func (d *Database) UserPlanQuotaDefaults(ctx context.Context, planID string, opts ...QueryOption) ([]PlanQuotaDefault, error) {
	var (
		err      error
		db       GoquDatabase
		defaults []PlanQuotaDefault
	)

	_, db = d.querySettings(opts...)

	pqdQuery := db.From(t.PQD).
		Select(
			t.PQD.Col("id").As("id"),
			t.PQD.Col("quota_value").As("quota_value"),
			t.PQD.Col("plan_id").As("plan_id"),
			t.RT.Col("id").As(goqu.C("resource_types.id")),
			t.RT.Col("name").As(goqu.C("resource_types.name")),
			t.RT.Col("unit").As(goqu.C("resource_types.unit")),
		).
		Join(t.RT, goqu.On(goqu.I("plan_quota_defaults.resource_type_id").Eq(goqu.I("resource_types.id")))).
		Where(t.PQD.Col("plan_id").Eq(planID)).
		Executor()

	if err = pqdQuery.ScanStructsContext(ctx, &defaults); err != nil {
		return nil, err
	}

	return defaults, nil
}

// UserPlanDetails returns lists of PlanQuotaDefaults, t.Quotas, and Usages
// Associated with the *UserPlan passed in. Accepts a variable number of
// QuotaOptions, though only WithTX is currently supported.
func (d *Database) UserPlanDetails(ctx context.Context, userPlan *UserPlan, opts ...QueryOption) ([]PlanQuotaDefault, []Quota, []Usage, error) {
	var (
		err      error
		defaults []PlanQuotaDefault
		usages   []Usage
		quotas   []Quota
	)

	log.Debug("before getting user plan quota defaults")
	defaults, err = d.UserPlanQuotaDefaults(ctx, userPlan.Plan.ID, opts...)
	if err != nil {
		return nil, nil, nil, err
	}
	log.Debug("after getting user plan quota defaults")

	log.Debug("before getting user plan t.Quotas")
	quotas, err = d.UserPlanQuotas(ctx, userPlan.ID, opts...)
	if err != nil {
		return nil, nil, nil, err
	}
	log.Debug("after getting user plan t.Quotas")

	log.Debug("before getting user plan usages")
	usages, err = d.UserPlanUsages(ctx, userPlan.ID, opts...)
	if err != nil {
		return nil, nil, nil, err
	}
	log.Debug("after getting user plan usages")

	return defaults, quotas, usages, nil
}
