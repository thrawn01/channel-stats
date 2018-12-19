package channelstats

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/mailgun/holster/slice"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

var (
	validParams    = []string{"start-hour", "end-hour", "channel", "counter"}
	requiredParams = []string{"channel", "counter"}
	validCounters  = []string{"messages", "positive", "negative", "link", "emoji", "word-count"}
)

const (
	listenAddr = "0.0.0.0:2020"
	staticPath = "html/"
)

type ParamDoc struct {
	Param string
	Desc  string
}

type EndpointDoc struct {
	Path   string
	Desc   string
	Params []ParamDoc
}

type CounterDoc struct {
	Counter string
	Desc    string
}

type DocResponse struct {
	Endpoints []EndpointDoc
	Counters  []CounterDoc
}

type ItemResp struct {
	StartHour string      `json:"start-hour"`
	EndHour   string      `json:"end-hour"`
	Items     interface{} `json:"items"`
}

type Server struct {
	idMgr  IDManager
	wg     sync.WaitGroup
	server *http.Server
	store  Storer
	log    *logrus.Entry
}

func NewServer(store Storer, idMgr IDManager) *Server {
	s := &Server{
		log:   GetLogger().WithField("prefix", "http"),
		idMgr: idMgr,
		store: store,
	}

	r := chi.NewRouter()

	// Middleware
	r.Use(NewStructuredLogger(s.log))
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(5 * time.Second))

	// UI Routes
	r.Get("/", s.redirectUI)
	r.Get("/index.html", s.redirectUI)
	r.Route("/ui", func(r chi.Router) {
		r.Get("/*", s.serveFiles)
	})

	// API routes
	r.Route("/api", func(r chi.Router) {
		r.Get("/all", s.getAll)
		r.Get("/", s.doc)
		r.Get("/datapoints", s.getDataPoints)
		r.Get("/sum", s.getSum)
		r.Get("/percentage", s.getPercentage)
		r.Get("/chart/sum", s.chartSum)
		r.Get("/chart/percentage", s.chartPercentage)
	})

	s.server = &http.Server{Addr: listenAddr, Handler: r}

	// Listen thingy
	s.wg.Add(1)
	go func() {
		s.log.Infof("Listening on %s", listenAddr)
		if err := s.server.ListenAndServe(); err != nil {
			if err != http.ErrServerClosed {
				s.log.Errorf("failed to bind to interface '%s': %s", listenAddr, err)
			}
		}
		s.wg.Done()
	}()
	return s
}

func (s *Server) Stop() error {
	err := s.server.Shutdown(context.Background())
	s.wg.Wait()
	return err
}

func (s *Server) doc(w http.ResponseWriter, r *http.Request) {
	resp := DocResponse{
		Endpoints: []EndpointDoc{
			{Path: "/api", Desc: "this index"},
			{
				Path: "/api/datapoints",
				Desc: "get the raw data points for the specific message counter",
				Params: []ParamDoc{
					{Param: "start-hour", Desc: "retrieve counters starting at this hour"},
					{Param: "end-hour", Desc: "retrieve counters ending at this hour"},
					{Param: "channel", Desc: "channel to retrieve counters for"},
					{Param: "counter", Desc: "name of the counter (See 'Counters' for valid counter names)"},
				},
			},
			{
				Path: "/api/sum",
				Desc: "sum the data points by user for a counter and channel",
				Params: []ParamDoc{
					{Param: "start-hour", Desc: "retrieve counters starting at this hour"},
					{Param: "end-hour", Desc: "retrieve counters ending at this hour"},
					{Param: "channel", Desc: "channel to retrieve counters for"},
					{Param: "counter", Desc: "name of the counter (See 'Counters' for valid counter names)"},
				},
			},
			{
				Path: "/api/percentage",
				Desc: "calculate what percentage of the counter makes up the total number messages",
				Params: []ParamDoc{
					{Param: "start-hour", Desc: "retrieve counters starting at this hour"},
					{Param: "end-hour", Desc: "retrieve counters ending at this hour"},
					{Param: "channel", Desc: "channel to retrieve counters for"},
					{Param: "counter", Desc: "name of the counter (See 'Counters' for valid counter names)"},
				},
			},
		},
		Counters: []CounterDoc{
			{Counter: "messages", Desc: "The number of messages seen in channel"},
			{Counter: "positive", Desc: "The number of messages that had positive sentiment seen in channel"},
			{Counter: "negative", Desc: "The number of messages that had negative sentiment seen in channel"},
			{Counter: "link", Desc: "The number of messages that contain an http link"},
			{Counter: "emoji", Desc: "The number of messages that contain an emoji link"},
			{Counter: "word-count", Desc: "The number of words counted in the channel"},
		},
	}
	toJSON(w, resp)
}

func (s *Server) getAll(w http.ResponseWriter, r *http.Request) {
	data, err := s.store.GetAll()
	if err != nil {
		abort(w, err, 500)
		return
	}
	toJSON(w, data)
}

func (s *Server) getDataPoints(w http.ResponseWriter, r *http.Request) {
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

	// Get the data points from the database
	data, err := s.store.GetDataPoints(
		timeRange,
		channelID,
		r.FormValue("counter"))

	if err != nil {
		abort(w, err, http.StatusInternalServerError)
		return
	}

	toJSON(w, ItemResp{
		StartHour: timeRange.StartDate(),
		EndHour:   timeRange.EndDate(),
		Items:     data,
	})
}

func (s *Server) getSum(w http.ResponseWriter, r *http.Request) {
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

	// aggregate the data points by user
	data, err := s.store.SumByUser(
		timeRange,
		channelID,
		r.FormValue("counter"))
	if err != nil {
		abort(w, err, http.StatusInternalServerError)
		return
	}
	toJSON(w, ItemResp{
		StartHour: timeRange.StartDate(),
		EndHour:   timeRange.EndDate(),
		Items:     data,
	})
}

func (s *Server) getPercentage(w http.ResponseWriter, r *http.Request) {
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

	results, err := s.store.PercentageByUser(timeRange, channelID, r.FormValue("counter"))
	if err != nil {
		abort(w, err, http.StatusInternalServerError)
		return
	}

	toJSON(w, ItemResp{
		StartHour: timeRange.StartDate(),
		EndHour:   timeRange.EndDate(),
		Items:     results,
	})
}

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

	w.Header().Set("Content-Type", "image/png")
	if err := RenderPercentage(s.store, w, timeRange, channelID, r.FormValue("counter")); err != nil {
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

	w.Header().Set("Content-Type", "image/png")
	if err := RenderSum(s.store, w, timeRange, channelID, r.FormValue("counter")); err != nil {
		abort(w, err, http.StatusInternalServerError)
	}
}

func abort(w http.ResponseWriter, err error, code int) {
	GetLogger().WithField("prefix", "http").Errorf("HTTP: %s\n", err)
	http.Error(w, err.Error(), code)
}

func toJSON(w http.ResponseWriter, obj interface{}) {
	resp, err := json.Marshal(obj)
	if err != nil {
		abort(w, errors.Wrap(err, "during JSON marshal"), 500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(resp)
}

func isValidParams(r *http.Request, validParams []string, requiredParams []string) error {
	if r.Form == nil {
		r.ParseMultipartForm(32 << 20)
	}

	paramsMap := make(map[string]bool)
	for key := range r.Form {
		paramsMap[key] = true
		if !slice.ContainsString(strings.ToLower(key), validParams, nil) {
			return fmt.Errorf("invalid parameter '%s'", key)
		}
	}

	for _, key := range requiredParams {
		if _, ok := paramsMap[key]; !ok {
			return fmt.Errorf("parameter '%s' is required", key)
		}
	}

	// If we are expecting a counter
	if slice.ContainsString("counter", requiredParams, nil) {
		// Should be one of the valid counters
		if !slice.ContainsString(r.Form.Get("counter"), validCounters, nil) {
			return fmt.Errorf("invalid 'counter' must be one of '%s'", strings.Join(validCounters, ","))
		}
	}
	return nil
}
