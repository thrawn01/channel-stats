package channelstats

import (
	"github.com/wcharczuk/go-chart"
	"github.com/wcharczuk/go-chart/drawing"
	"io"
	"sort"
)

func counterToColor(counter string) string {
	switch counter {
	case "messages":
		return "blue"
	case "positive":
		return "green"
	case "negative":
		return "red"
	case "emoji":
		return "yellow"
	default:
		return "blue"
	}
}

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

	return renderBarChart(w, dps, counterToColor(counter))
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

	return renderBarChart(w, dps, counterToColor(counter))
}

func renderBarChart(w io.Writer, bars []chart.Value, color string) error {

	sbc := chart.BarChart{
		ColorPalette: CustomColors{Colors: barColors[color]},
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

type CustomColors struct {
	Colors []drawing.Color
}

func (c CustomColors) BackgroundColor() drawing.Color {
	return chart.DefaultBackgroundColor
}

func (c CustomColors) BackgroundStrokeColor() drawing.Color {
	return chart.DefaultBackgroundStrokeColor
}

func (c CustomColors) CanvasColor() drawing.Color {
	return chart.DefaultCanvasColor
}

func (c CustomColors) CanvasStrokeColor() drawing.Color {
	return chart.DefaultCanvasStrokeColor
}

func (c CustomColors) AxisStrokeColor() drawing.Color {
	return chart.DefaultAxisColor
}

func (c CustomColors) TextColor() drawing.Color {
	return chart.DefaultTextColor
}

func (c CustomColors) GetSeriesColor(index int) drawing.Color {
	finalIndex := index % len(c.Colors)
	return c.Colors[finalIndex]
}

var (
	barColors = map[string][]drawing.Color{
		"red": {
			{R: 237, G: 192, B: 198, A: 255}, // Light
			{R: 219, G: 136, B: 141, A: 255},
			{R: 203, G: 86, B: 100, A: 255},
			{R: 208, G: 2, B: 27, A: 255}, // Dark
		},
		"green": {
			{R: 209, G: 239, B: 255, A: 255}, // Light
			{R: 167, G: 223, B: 199, A: 255},
			{R: 129, G: 207, B: 170, A: 255},
			{R: 73, G: 178, B: 121, A: 255}, // Dark
		},
		"blue": {
			{R: 212, G: 217, B: 236, A: 255}, // Light
			{R: 168, G: 180, B: 217, A: 255},
			{R: 130, G: 146, B: 201, A: 255},
			{R: 60, G: 83, B: 167, A: 255}, // Dark
		},
		"yellow": {
			{R: 252, G: 242, B: 212, A: 255}, // Light
			{R: 250, G: 230, B: 170, A: 255},
			{R: 246, G: 217, B: 134, A: 255},
			{R: 241, G: 192, B: 69, A: 255}, // Dark
		},
	}
)
