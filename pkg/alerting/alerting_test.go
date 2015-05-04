package alerting

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"bosun.org/graphite"
	. "github.com/smartystreets/goconvey/convey"
)

type fakeGraphite struct {
	resp graphite.Response
}

func (f *fakeGraphite) Query(r *graphite.Request) (graphite.Response, error) {
	return f.resp, nil
}

func NewFakeGraphiteWithTestValue(val int) *fakeGraphite {
	fg := &fakeGraphite{}
	fg.resp = graphite.Response{
		graphite.Series{
			Target: "test",
			Datapoints: []graphite.DataPoint{
				graphite.DataPoint{json.Number(fmt.Sprintf("%d", val)), json.Number("1234567890")},
			},
		},
	}
	return fg
}

func TestAlerting(t *testing.T) {

	Convey("when evaluating graphite checks", t, func() {
		Convey("Given a graphite check expression", func() {
			checkDef := CheckDef{
				CritExpr: `median(graphite("test", "2m", "", "")) > 100`,
			}

			Convey("Series median above threshold should trigger alert", func() {
				fg := NewFakeGraphiteWithTestValue(150)
				evaluator, err := NewGraphiteCheckEvaluator(fg, checkDef)
				So(err, ShouldBeNil)

				res, err := evaluator.Eval(time.Now())
				So(err, ShouldBeNil)
				So(res, ShouldEqual, EvalResultCrit)
			})

			Convey("Series median equal to threshold should not trigger alert", func() {
				fg := NewFakeGraphiteWithTestValue(100)
				evaluator, err := NewGraphiteCheckEvaluator(fg, checkDef)
				So(err, ShouldBeNil)

				res, err := evaluator.Eval(time.Now())
				So(err, ShouldBeNil)
				So(res, ShouldEqual, EvalResultOK)
			})

			Convey("Series median below threshold should not trigger alert", func() {
				fg := NewFakeGraphiteWithTestValue(70)
				evaluator, err := NewGraphiteCheckEvaluator(fg, checkDef)
				So(err, ShouldBeNil)

				res, err := evaluator.Eval(time.Now())
				So(err, ShouldBeNil)
				So(res, ShouldEqual, EvalResultOK)
			})
		})

		Convey("Given two graphite check expressions", func() {
			checkDef := CheckDef{
				WarnExpr: `median(graphite("test", "2m", "", "")) < 150`,
				CritExpr: `median(graphite("test", "2m", "", "")) < 100`,
			}

			Convey("Series median in critical region", func() {
				fg := NewFakeGraphiteWithTestValue(99)
				evaluator, err := NewGraphiteCheckEvaluator(fg, checkDef)
				So(err, ShouldBeNil)

				res, err := evaluator.Eval(time.Now())
				So(err, ShouldBeNil)
				So(res, ShouldEqual, EvalResultCrit)
			})

			Convey("Series median in warn region", func() {
				fg := NewFakeGraphiteWithTestValue(100)
				evaluator, err := NewGraphiteCheckEvaluator(fg, checkDef)
				So(err, ShouldBeNil)

				res, err := evaluator.Eval(time.Now())
				So(err, ShouldBeNil)
				So(res, ShouldEqual, EvalResultWarn)
			})

			Convey("Series median in OK region", func() {
				fg := NewFakeGraphiteWithTestValue(150)
				evaluator, err := NewGraphiteCheckEvaluator(fg, checkDef)
				So(err, ShouldBeNil)

				res, err := evaluator.Eval(time.Now())
				So(err, ShouldBeNil)
				So(res, ShouldEqual, EvalResultOK)
			})
		})
	})
}
