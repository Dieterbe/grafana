package alerting

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"bosun.org/graphite"
	. "github.com/smartystreets/goconvey/convey"
)

func NewFakeGraphite(values [][]int, initialTs int64, step int) *fakeGraphite {
	fg := &fakeGraphite{}
    series := make([]graphite.Series, 0)
    for serieNum, pointSlice := range values {
        serie := graphite.Series{
            Target : fmt.Sprintf("test.serie.%d", serieNum),
        }
        for i, point := range pointSlice {
            serie.Datapoints = append(serie.Datapoints, graphite.DataPoint{
                json.Number(fmt.Sprintf("%d", point)),
                json.Number(fmt.Sprintf("%d", int(initialTs) + i*step)),
            })
        }
        series = append(series, serie)
    }

	fg.resp = graphite.Response(series)
	return fg
}

func check(expr string, warn, crit int, values [][]int, expectErr error, expectRes CheckEvalResult) {
        checkDef := CheckDef{}
        if warn != -1 {
            checkDef.WarnExpr = fmt.Sprintf(expr, `graphite("test", "2m", "", "")`, warn)
        }
        if crit != -1 {
            checkDef.CritExpr = fmt.Sprintf(expr, `graphite("test", "2m", "", "")`, crit)
        }
        now := time.Now()
        end := now.Unix()
        step := 10
        steps := 0
        for _, serie := range values {
            if len(serie) > steps {
                steps = len(serie)
            }
        }
        fmt.Printf("vals %v - end %d, steps %d\n", values, end, steps)
        fg := NewFakeGraphite(values, end - int64((steps-1)*step), steps)
        evaluator, err := NewGraphiteCheckEvaluator(fg, checkDef)
        So(err, ShouldBeNil)

        res, err := evaluator.Eval(now)
        So(err, ShouldEqual, expectErr)
        So(res, ShouldEqual, expectRes)
    }

func TestAlertingMinimal(t *testing.T) {

	Convey("check result on 1 series with 1 point should match expected outcome", t, func() {
        check(`median(%s) > %d`, -1, 100, [][]int{[]int{150}}, nil, EvalResultCrit)
        check(`median(%s) > %d`, -1, 100, [][]int{[]int{100}}, nil, EvalResultOK)
        check(`median(%s) > %d`, -1, 100, [][]int{[]int{70}}, nil, EvalResultOK)
        check(`median(%s) < %d`, 150, 100, [][]int{[]int{70}}, nil, EvalResultCrit)
        check(`median(%s) < %d`, 150, 100, [][]int{[]int{100}}, nil, EvalResultWarn)
        check(`median(%s) < %d`, 150, 100, [][]int{[]int{150}}, nil, EvalResultOK)
        check(`median(%s) < %d`, 150, 100, [][]int{[]int{200}}, nil, EvalResultOK)
    })

	Convey("check result on 1 series with 3 points should match expected outcome", t, func() {
        check(`median(%s) > %d`, -1, 100, [][]int{[]int{50, 150, 200}}, nil, EvalResultCrit)
        check(`median(%s) > %d`, -1, 100, [][]int{[]int{50, 100, 150}}, nil, EvalResultOK)
        check(`median(%s) > %d`, -1, 100, [][]int{[]int{20, 70, 120}}, nil, EvalResultOK)
        check(`median(%s) < %d`, 150, 100, [][]int{[]int{20, 70, 120}}, nil, EvalResultCrit)
        check(`median(%s) < %d`, 150, 100, [][]int{[]int{50, 100, 150}}, nil, EvalResultWarn)
        check(`median(%s) < %d`, 150, 100, [][]int{[]int{100, 150, 200}}, nil, EvalResultOK)
        check(`median(%s) < %d`, 150, 100, [][]int{[]int{150, 200, 250}}, nil, EvalResultOK)
    })

	Convey("check result on 3 series with 1 point each should match the worst series", t, func() {
        check(`median(%s) > %d`, -1, 100, [][]int{[]int{50}, []int{150}, []int{200}}, nil, EvalResultCrit)
        check(`median(%s) > %d`, -1, 100, [][]int{[]int{50}, []int{100}, []int{150}}, nil, EvalResultCrit)
        check(`median(%s) > %d`, -1, 100, [][]int{[]int{20}, []int{70}, []int{120}}, nil, EvalResultCrit)
        check(`median(%s) > %d`, -1, 100, [][]int{[]int{10}, []int{50}, []int{100}}, nil, EvalResultOK)
        check(`median(%s) > %d`, -1, 100, [][]int{[]int{10}, []int{10}, []int{80}}, nil, EvalResultOK)
        check(`median(%s) > %d`, -1, 100, [][]int{[]int{10}, []int{10}, []int{60}}, nil, EvalResultOK)
        check(`median(%s) > %d`, -1, 100, [][]int{[]int{10}, []int{101}, []int{50}}, nil, EvalResultCrit)
        check(`median(%s) < %d`, 150, 100, [][]int{[]int{20}, []int{70}, []int{120}}, nil, EvalResultCrit)
        check(`median(%s) < %d`, 150, 100, [][]int{[]int{50}, []int{100}, []int{150}}, nil, EvalResultCrit)
        check(`median(%s) < %d`, 150, 100, [][]int{[]int{100}, []int{150}, []int{200}}, nil, EvalResultWarn)
        check(`median(%s) < %d`, 150, 100, [][]int{[]int{150}, []int{200}, []int{250}}, nil, EvalResultOK)
    })
}
