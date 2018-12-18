package channelstats

import (
	"github.com/wcharczuk/go-chart"
	"io"
	"sort"
)

func RenderPercentage(store Storer, w io.Writer, timeRange *TimeRange, channelID, counter string) error {
	totals, err := store.PercentageByUser(timeRange, channelID, counter)
	if err != nil {
		return err
	}

	var dps []chart.Value

	// Get at most 4 bars of data, if there are less than 4 use length instead
	offset := len(totals) - 4
	if offset < 1 {
		offset = len(totals)
	}

	for _, item := range totals[offset:] {
		dps = append(dps, chart.Value{Label: item.User, Value: float64(item.Percent)})
	}

	sort.Slice(dps, func(i, j int) bool {
		return dps[i].Value < dps[j].Value
	})

	return renderBarChart(w, dps)
}

func RenderSum(store Storer, w io.Writer, timeRange *TimeRange, channelID, counter string) error {
	totals, err := store.SumByUser(timeRange, channelID, counter)
	if err != nil {
		return err
	}

	var dps []chart.Value

	// Get at most 4 bars of data, if there are less than 4 use length instead
	offset := len(totals) - 4
	if offset < 1 {
		offset = len(totals)
	}

	for _, item := range totals[offset:] {
		dps = append(dps, chart.Value{Label: item.User, Value: float64(item.Sum)})
	}

	return renderBarChart(w, dps)
}

func renderBarChart(w io.Writer, bars []chart.Value) error {
	sbc := chart.BarChart{
		Background: chart.Style{
			Show: true,
			Padding: chart.Box{
				Top:    10,
				Right:  15,
				Bottom: 25,
				IsSet:  true,
			},
		},
		Height:   300,
		Width:    500,
		BarWidth: 65,
		XAxis: chart.Style{
			Show:     true,
			FontSize: 13,
			//TextHorizontalAlign: chart.Text,
			//TextVerticalAlign: chart.TextVerticalAlignBottom,
			//TextWrap:          chart.TextWrapNone,
		},
		YAxis: chart.YAxis{
			Style: chart.Style{
				Show:     true,
				FontSize: 12,
			},
			ValueFormatter: FloatFormatter,
		},
		Bars: bars,
	}
	return sbc.Render(chart.PNG, w)
}

func FloatFormatter(v interface{}) string {
	return chart.FloatValueFormatterWithFormat(v, "%.f")
}
