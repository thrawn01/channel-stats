package channelstats

import (
	"fmt"
	"github.com/wcharczuk/go-chart"
	"net/http"
)

func (s *Server) chart(w http.ResponseWriter, r *http.Request) {
	sbc := chart.BarChart{
		//Title:      "Test Bar Chart",
		//TitleStyle: chart.StyleShow(),
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
			FontSize: 10,
		},
		YAxis: chart.YAxis{
			/*TickStyle: chart.Style{
				Show:        true,
				StrokeColor: chart.ColorBlack,
				StrokeWidth: 10,
			},
			GridMajorStyle: chart.Style{
				Show:        true,
				StrokeColor: chart.ColorBlack,
				StrokeWidth: 10,
			},
			GridMinorStyle: chart.Style{
				Show:        true,
				StrokeColor: chart.ColorBlack,
				StrokeWidth: 10,
			},*/
			Style: chart.Style{
				Show:     true,
				FontSize: 10,
			},
		},
		Bars: []chart.Value{
			{Value: 10, Label: "Blue Green"},
			{Value: 100, Label: "Green"},
			{Value: 200, Label: "Gray"},
			{Value: 400, Label: "Orange"},
		},
	}

	w.Header().Set("Content-Type", "image/png")
	err := sbc.Render(chart.PNG, w)
	if err != nil {
		fmt.Printf("Error rendering chart: %v\n", err)
	}
}
