package channelstats

import (
	"github.com/wcharczuk/go-chart"
	"io"
	"net/http"
	"sort"
)

func (s *Server) chartPercentage(w http.ResponseWriter, r *http.Request) {
	if err := isValidParams(r, validParams, requiredParams); err != nil {
		abort(w, err, http.StatusBadRequest)
		return
	}

	channelID, err := s.idMgr.GetChannelID(r.FormValue("channel"))
	if err != nil {
		abort(w, err, http.StatusBadRequest)
		return
	}

	timeRange, err := NewTimeRange(r.FormValue("start-hour"), r.FormValue("end-hour"))
	if err != nil {
		abort(w, err, http.StatusBadRequest)
		return
	}

	totals, err := s.store.PercentageByUser(timeRange, r.FormValue("counter"), channelID)
	if err != nil {
		abort(w, err, http.StatusBadRequest)
		return
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

	w.Header().Set("Content-Type", "image/png")
	if err := s.renderBarChart(w, dps); err != nil {
		abort(w, err, http.StatusInternalServerError)
	}
}

func (s *Server) chartSum(w http.ResponseWriter, r *http.Request) {
	if err := isValidParams(r, validParams, requiredParams); err != nil {
		abort(w, err, http.StatusBadRequest)
		return
	}

	channelID, err := s.idMgr.GetChannelID(r.FormValue("channel"))
	if err != nil {
		abort(w, err, http.StatusBadRequest)
		return
	}

	timeRange, err := NewTimeRange(r.FormValue("start-hour"), r.FormValue("end-hour"))
	if err != nil {
		abort(w, err, http.StatusBadRequest)
		return
	}

	totals, err := s.store.SumByUser(timeRange, r.FormValue("counter"), channelID)
	if err != nil {
		abort(w, err, http.StatusBadRequest)
		return
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

	w.Header().Set("Content-Type", "image/png")
	if err := s.renderBarChart(w, dps); err != nil {
		abort(w, err, http.StatusInternalServerError)
	}
}

func (s *Server) renderBarChart(w io.Writer, bars []chart.Value) error {
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
	return sbc.Render(chart.PNG, w)
}
