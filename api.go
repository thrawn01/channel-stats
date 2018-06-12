package channelstats

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

const (
	listenAddr          = "0.0.0.0:2020"
	missingChannelIDErr = "missing 'channel-id' from request context"
	missingFileID       = "missing 'channel-id' from request context"
	staticPath          = "html/"
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

type ItemResponse struct {
	StartHour string      `json:"start-hour"`
	EndHour   string      `json:"end-hour"`
	Items     interface{} `json:"items"`
}

type Server struct {
	idMgr  *IDManager
	wg     sync.WaitGroup
	server *http.Server
	store  *Store
	log    *logrus.Entry
}

func NewServer(store *Store, idMgr *IDManager) *Server {
	s := &Server{
		log:   log.WithField("prefix", "http"),
		idMgr: idMgr,
		store: store,
	}

	r := chi.NewRouter()

	// Middleware
	r.Use(middleware.Logger)
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
		r.Route("/channels/{channel}", func(r chi.Router) {
			r.Use(s.channelToID)
			r.Get("/data/{counter}", s.getDataPoints)
			r.Get("/sum/{counter}", s.getSum)
		})
	})

	s.server = &http.Server{Addr: listenAddr, Handler: r}

	// Listen thingy
	s.wg.Add(1)
	go func() {
		s.log.Infof("Listening on %s", listenAddr)
		if err := s.server.ListenAndServe(); err != nil {
			if err != http.ErrServerClosed {
				log.Errorf("failed to bind to interface '%s': %s", listenAddr, err)
			}
		}
		s.wg.Done()
	}()
	return s
}

func (s *Server) Stop() {
	s.server.Shutdown(context.Background())
	s.wg.Wait()
}

func (s *Server) doc(w http.ResponseWriter, r *http.Request) {
	resp := DocResponse{
		Endpoints: []EndpointDoc{
			{Path: "/api", Desc: "this index"},
			{
				Path: "/api/channels/{channel}/data/{counter}",
				Desc: "raw data points for the specific message counter",
				Params: []ParamDoc{
					{Param: "start-hour", Desc: "retrieve counters starting at this hour"},
					{Param: "end-hour", Desc: "retrieve counters ending at this hour"},
				},
			},
			{
				Path: "/api/channels/{channel}/sum/{counter}",
				Desc: "sum the data points by user for a counter and channel",
				Params: []ParamDoc{
					{Param: "start-hour", Desc: "retrieve counters starting at this hour"},
					{Param: "end-hour", Desc: "retrieve counters ending at this hour"},
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

	// Get the data points from the database
	data, err := s.store.GetDataPoints(
		timeRange,
		chi.URLParam(r, "counter"),
		channelID)

	if err != nil {
		abort(w, err, http.StatusInternalServerError)
		return
	}

	toJSON(w, ItemResponse{
		StartHour: timeRange.StartDate(),
		EndHour:   timeRange.EndDate(),
		Items:     data,
	})
}

func (s *Server) getSum(w http.ResponseWriter, r *http.Request) {
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

	// aggregate the data points by user
	data, err := s.store.SumByUser(
		timeRange,
		chi.URLParam(r, "counter"),
		channelID)
	if err != nil {
		abort(w, err, http.StatusInternalServerError)
		return
	}
	toJSON(w, ItemResponse{
		StartHour: timeRange.StartDate(),
		EndHour:   timeRange.EndDate(),
		Items:     data,
	})
}

// Middleware to convert channel name to channel id and place the result in the request context
func (s *Server) channelToID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := chi.URLParam(r, "channel")
		if name == "" {
			abort(w, errors.New("'channel' missing from request"), http.StatusBadRequest)
			return
		}
		id, err := s.idMgr.GetChannelID(name)
		if err != nil {
			abort(w, err, http.StatusBadRequest)
			return
		}
		ctx := context.WithValue(r.Context(), "channel-id", id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func abort(w http.ResponseWriter, err error, code int) {
	log.WithField("prefix", "http").Errorf("HTTP: %s\n", err)
	http.Error(w, http.StatusText(code), code)
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
