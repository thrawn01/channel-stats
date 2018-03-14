package channelstats

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
	"github.com/pkg/errors"
)

const addr = "0.0.0.0:1313"

type Endpoint struct {
	Path string
	Desc string
}

type Server struct {
	store  *Store
	server *http.Server
	wg     sync.WaitGroup
}

func NewServer(store *Store) *Server {
	// Server
	s := &Server{
		store: store,
	}

	r := chi.NewRouter()

	// Middleware
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(60 * time.Second))

	// Routes
	r.Get("/", s.index)
	r.Get("/all", s.getAll)
	r.Get("/datapoints", s.getDataPoints)
	//r.Get("/message/totals/{channel-id}", s.getMessageTotals)

	s.server = &http.Server{Addr: addr, Handler: r}

	// Listen thingy
	s.wg.Add(1)
	go func() {
		fmt.Printf("-- Listening on %s\n", addr)
		if err := s.server.ListenAndServe(); err != nil {
			if err != http.ErrServerClosed {
				fmt.Fprintf(os.Stderr, "during ListenAndServe(): %s\n", err)
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
		{Path: "/datapoints", Desc: "retrieve raw datapoints from the key value store"},
		{Path: "/message/totals/{channel-id}", Desc: "total number of messages by user for a channel"},
		{Path: "/sentiment/positive/{channel-id}", Desc: "total number of positive sentiment messages by user for a channel"},
		{Path: "/sentiment/negative/{channel-id}", Desc: "total number of negative sentiment messages by user for a channel"},
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
	timeRange, err := NewTimeRange(r.FormValue("from"), r.FormValue("to"))
	if err != nil {
		abort(w, err, 500)
		return
	}

	// Get the datapoints from the database
	data, err := s.store.GetDataPoints(
		timeRange,
		"messages",
		chi.URLParam(r, "channel-id"))

	if err != nil {
		abort(w, err, 500)
		return
	}
	toJSON(w, data)
}

/*func (s *Server) getMessageTotals(w http.ResponseWriter, r *http.Request) {
	// aggregate the datapoints by user
	data, err := s.store.SumByUser(
		r.FormValue("from"),
		r.FormValue("to"),
		chi.URLParam(r, "channel-id"),
		"messages")
	if err != nil {
		abort(w, err, 500)
		return
	}

	toJSON(w, data)
}*/

/*	hour := time.Now().Format(hourLayout)
	keyPrefix := DpKey(hour, channelID, "messages")

	err := s.db.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()
		for it.Seek(keyPrefix); it.ValidForPrefix(keyPrefix); it.Next() {
			item := it.Item()
			k := item.Key()
			v, err := item.Value()
			if err != nil {
				return err
			}
			response.Items = append(response.Items, Pair{Key: string(k), Value: string(v)})
		}
		return nil
	})
	if err != nil {
		abort(w, errors.Wrap(err, "during database view"), 500)
		return
	}
*/

func abort(w http.ResponseWriter, err error, code int) {
	fmt.Fprint(os.Stderr, "", err)
	http.Error(w, http.StatusText(500), 500)
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
