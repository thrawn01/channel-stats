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
)

const (
	listenAddr          = "0.0.0.0:2020"
	missingChannelIDErr = "missing 'channel-id' from request context"
)

type Endpoint struct {
	Path string
	Desc string
}

type Server struct {
	chanMgr *ChannelManager
	wg      sync.WaitGroup
	server  *http.Server
	store   *Store
}

func NewServer(store *Store, chanMgr *ChannelManager) *Server {
	s := &Server{
		chanMgr: chanMgr,
		store:   store,
	}

	r := chi.NewRouter()

	// Middleware
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(5 * time.Second))

	// Routes
	r.Get("/", s.index)
	r.Get("/all", s.getAll)

	r.Route("/channels/{channel}", func(r chi.Router) {
		r.Use(s.channelToID)
		r.Get("/data/{type}", s.getDataPoints)
		r.Get("/sum/{type}", s.getSum)
	})

	s.server = &http.Server{Addr: listenAddr, Handler: r}

	// Listen thingy
	s.wg.Add(1)
	go func() {
		log.Infof("Listening on %s", listenAddr)
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

func (s *Server) index(w http.ResponseWriter, r *http.Request) {
	response := []Endpoint{
		{Path: "/", Desc: "this index"},
		{Path: "/channels/{channel}/data/{type}", Desc: "raw data points for the specific message type"},
		{Path: "/channels/{channel}/sum/{type}", Desc: "sum the data points by user for a type and channel"},
	}
	toJSON(w, response)
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

	timeRange, err := NewTimeRange(r.FormValue("from"), r.FormValue("to"))
	if err != nil {
		abort(w, err, http.StatusBadRequest)
		return
	}

	// Get the data points from the database
	data, err := s.store.GetDataPoints(
		timeRange,
		chi.URLParam(r, "type"),
		channelID)

	if err != nil {
		abort(w, err, http.StatusInternalServerError)
		return
	}
	toJSON(w, data)
}

func (s *Server) getSum(w http.ResponseWriter, r *http.Request) {
	channelID, ok := r.Context().Value("channel-id").(string)
	if !ok {
		abort(w, errors.New(missingChannelIDErr), http.StatusBadRequest)
		return
	}

	timeRange, err := NewTimeRange(r.FormValue("from"), r.FormValue("to"))
	if err != nil {
		abort(w, err, http.StatusBadRequest)
		return
	}

	// aggregate the data points by user
	data, err := s.store.SumByUser(
		timeRange,
		chi.URLParam(r, "type"),
		channelID)
	if err != nil {
		abort(w, err, http.StatusInternalServerError)
		return
	}
	toJSON(w, data)
}

// Convert channel names to channel id and place the result in the request context
func (s *Server) channelToID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := chi.URLParam(r, "channel")
		if name == "" {
			abort(w, errors.New("'channel' missing from request"), http.StatusBadRequest)
			return
		}
		id, err := s.chanMgr.GetID(name)
		if err != nil {
			abort(w, err, http.StatusBadRequest)
			return
		}
		ctx := context.WithValue(r.Context(), "channel-id", id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func abort(w http.ResponseWriter, err error, code int) {
	log.Errorf("HTTP: %s\n", err)
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
