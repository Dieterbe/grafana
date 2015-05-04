package alerting

import (
	"bosun.org/cmd/bosun/cache"
	"bosun.org/cmd/bosun/expr"
	"bosun.org/graphite"
	"fmt"
	"time"
)

type CheckDef struct {
	// Freq      uint32
	// Offset    uint8 // offset on top of "even" minute/10s/.. intervals
	// Expr      string // "median(foo...) "
	// LevelWarn float64 // > 5 , < 10
	// LevelCrit float64 // > 10, < 5
	CritExpr string
	WarnExpr string
}

type Check struct {
	Id           int64
	OrgId        int64
	DataSourceId int64
	Definition   CheckDef
}

// func (check *Check) getDataSource() {
// 	dsQuery := m.GetDataSourceByIdQuery{Id: check.Id}
//
// 	if err := bus.Dispatch(&dsQuery); err != nil {
// 		return nil, err
// 	}
//
// 	return dsQuery.Result
// }

type CheckEvalResult int

const (
	EvalResultOK CheckEvalResult = 1 << iota
	EvalResultWarn
	EvalResultCrit
	EvalResultUnknown
)

func (c CheckEvalResult) String() string {
	switch c {
	case EvalResultOK:
		return "OK"
	case EvalResultWarn:
		return "Warning"
	case EvalResultCrit:
		return "Critical"
	case EvalResultUnknown:
		return "Unknown"
	default:
		panic(fmt.Sprintf("Invalid CheckEvalResult value %d", int(c)))
	}
}

type CheckEvaluator interface {
	Eval() (*CheckEvalResult, error)
}

type GraphiteCheckEvaluator struct {
	Context  graphite.Context
	Check    CheckDef
	critExpr *expr.Expr
	warnExpr *expr.Expr
}

func NewGraphiteCheckEvaluator(c graphite.Context, check CheckDef) (*GraphiteCheckEvaluator, error) {
	var warnExpr *expr.Expr
	var critExpr *expr.Expr
	var err error
	if check.WarnExpr != "" {
		warnExpr, err = expr.New(check.WarnExpr, expr.Graphite)
		if err != nil {
			return nil, err
		}
	}
	if check.CritExpr != "" {
		critExpr, err = expr.New(check.CritExpr, expr.Graphite)
		if err != nil {
			return nil, err
		}
	}
	return &GraphiteCheckEvaluator{
		Context:  c,
		Check:    check,
		warnExpr: warnExpr,
		critExpr: critExpr,
	}, nil
}

func (ce *GraphiteCheckEvaluator) Eval(ts time.Time) (CheckEvalResult, error) {
	// create cache
	// this is so that when bosun queries the same graphite query multiple times
	// like in (median(graphite("foo", "2m", "",""))> 10 || avg(graphite("foo", "2m", "","")) > 20)
	// it reuses the same resultsets internally.
	// cache is unbounded so that we are guaranteed consistent results
	cacheObj := cache.New(0)

	if ce.critExpr != nil {
		results, _, err := ce.critExpr.Execute(nil, ce.Context, nil, cacheObj, nil, ts, 0, true, nil, nil, nil)
		if err != nil {
			return EvalResultUnknown, err
		}
		for _, res := range results.Results {
			if int(res.Value.Value().(expr.Number)) > 0 {
				return EvalResultCrit, nil
			}
		}
	}
	if ce.warnExpr != nil {
		results, _, err := ce.warnExpr.Execute(nil, ce.Context, nil, cacheObj, nil, ts, 0, true, nil, nil, nil)
		if err != nil {
			return EvalResultUnknown, err
		}
		for _, res := range results.Results {
			if int(res.Value.Value().(expr.Number)) > 0 {
				return EvalResultWarn, nil
			}
		}
	}

	return EvalResultOK, nil
}
