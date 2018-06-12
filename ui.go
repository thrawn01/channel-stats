package channelstats

import (
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"

	"github.com/go-chi/chi"
	"github.com/pkg/errors"
)

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
