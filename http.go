package channelstats

import (
	"context"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"fmt"

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

type Endpoint struct {
	Path string
	Desc string
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
		r.Get("/", s.api)
		r.Route("/channels/{channel}", func(r chi.Router) {
			r.Use(s.channelToID)
			r.Get("/data/{type}", s.getDataPoints)
			r.Get("/sum/{type}", s.getSum)
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

func (s *Server) serveFiles(w http.ResponseWriter, r *http.Request) {
	file := chi.URLParam(r, "*")
	if file == "" {
		abort(w, errors.New("'*' param missing from request"), http.StatusBadRequest)
		return
	}

	path := fmt.Sprintf("%s%s", staticPath, file)
	ext := filepath.Ext(path)

	// Ignore chrome .map files
	if ext == ".map" {
		http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
		return
	}

	// Determine our content type by file extension
	ctype := mime.TypeByExtension(filepath.Ext(path))
	if ctype == "" {
		s.log.Debug("Unable to determine mime type for ", path)
		w.Header().Set("Content-Type", "text/html")
	} else {
		w.Header().Set("Content-Type", ctype)
	}

	// Open the requested file
	fd, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
			return
		}
		if os.IsPermission(err) {
			abort(w, err, http.StatusForbidden)
			return
		}
		abort(w, err, http.StatusInternalServerError)
		return
	}
	defer fd.Close()

	// Write the entire file back to the client
	io.Copy(w, fd)
}

func (s *Server) redirectUI(resp http.ResponseWriter, req *http.Request) {
	s.log.Debug("Redirect to '/ui/index.html'")
	resp.Header().Set("Location", "/ui/index.html")
	resp.WriteHeader(http.StatusMovedPermanently)
}

func (s *Server) api(w http.ResponseWriter, r *http.Request) {
	response := []Endpoint{
		{Path: "/api", Desc: "this index"},
		{Path: "/api/channels/{channel}/data/{type}", Desc: "raw data points for the specific message type"},
		{Path: "/api/channels/{channel}/sum/{type}", Desc: "sum the data points by user for a type and channel"},
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

	timeRange, err := NewTimeRange(r.FormValue("start"), r.FormValue("end"))
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

	timeRange, err := NewTimeRange(r.FormValue("start"), r.FormValue("end"))
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
