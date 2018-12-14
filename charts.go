package channelstats

import (
	"github.com/go-chi/chi"
	"github.com/pkg/errors"
	"github.com/wcharczuk/go-chart"
	"net/http"
)

func (s *Server) chart(w http.ResponseWriter, r *http.Request) {
	if err := isValidParams(r, validGetParams); err != nil {
		abort(w, err, http.StatusBadRequest)
		return
	}

	channelID, ok := r.Context().Value("channel-id").(string)
	if !ok {
		abort(w, errors.New(missingChannelIDErr), http.StatusBadRequest)
		return
	}

	timeRange, err := NewTimeRange(r.FormValue("start-hour"), r.FormValue("end-hour"))
	if err != nil {
		abort(w, err, http.StatusBadRequest)
		return
	}

	if err := s.renderBarChart(w, timeRange, chi.URLParam(r, "counter"), channelID); err != nil {
		abort(w, err, http.StatusInternalServerError)
	}
}

func (s *Server) renderBarChart(w http.ResponseWriter, tr *TimeRange, cType, cID string) error {
	data, err := s.store.SumByUser(tr, cType, cID)
	if err != nil {
		return err
	}

	var bars []chart.Value
	for _, item := range data[len(data)-4:] {
		bars = append(bars, chart.Value{Label: item.User, Value: float64(item.Sum)})
	}

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
		},
		YAxis: chart.YAxis{
			Style: chart.Style{
				Show:     true,
				FontSize: 12,
			},
		},
		Bars: bars,
	}

	w.Header().Set("Content-Type", "image/png")
	return sbc.Render(chart.PNG, w)
}
