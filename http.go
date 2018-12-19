package channelstats

import (
	"bytes"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/go-chi/chi"
	"github.com/pkg/errors"
	"github.com/thrawn01/channel-stats/html"
)

func (s *Server) serveFiles(w http.ResponseWriter, r *http.Request) {
	file := chi.URLParam(r, "*")
	if file == "" || file == "index.html" {
		s.redirectUI(w, r)
		return
	}

	if file == "index" {
		s.serveIndex(w, r)
		return
	}

	filePath := fmt.Sprintf("%s%s", staticPath, file)
	ext := filepath.Ext(filePath)

	// Ignore chrome .map files
	if ext == ".map" {
		http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
		return
	}

	// Determine our content type by file extension
	ctype := mime.TypeByExtension(filepath.Ext(filePath))
	if ctype == "" {
		s.log.Debug("Unable to determine mime type for ", filePath)
		w.Header().Set("Content-Type", "text/html")
	} else {
		w.Header().Set("Content-Type", ctype)
	}

	// Fetch the files from the asset store
	buf, err := html.Get(filePath)
	if err != nil {
		if ioErr, ok := err.(*os.PathError); ok {
			if os.IsNotExist(ioErr) {
				http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
				return
			}
			if os.IsPermission(ioErr) {
				abort(w, err, http.StatusForbidden)
				return
			}
		}
		abort(w, err, http.StatusInternalServerError)
		return
	}

	// Write the entire file back to the client
	if _, err := io.Copy(w, bytes.NewBuffer(buf)); err != nil {
		abort(w, err, http.StatusInternalServerError)
	}
}

type TemplateData struct {
	PageTitle   string
	PageHours   string
	GraphParams string
}

func (s *Server) serveIndex(w http.ResponseWriter, r *http.Request) {
	// Get a list of channels the bot has joined and is listening on
	channels := filterOnlyMembers(s.idMgr.Channels())

	timeRange, err := NewTimeRange(r.FormValue("start-hour"), r.FormValue("end-hour"))
	if err != nil {
		abort(w, err, http.StatusBadRequest)
		return
	}

	channelName := r.FormValue("channel")
	if channelName != "" {
		if err := isValidChannel(channelName, channels); err != nil {
			abort(w, err, http.StatusBadRequest)
			return
		}
	} else {
		// Pick the first
		if len(channels) == 0 {
			abort(w, errors.New("Bot has not joined any public channels"+
				", invite the bot to a channel in slack!"), http.StatusBadRequest)
			return
		}
		channelName = channels[0].Name
		r.Form["channel"] = []string{channelName}
	}

	data := TemplateData{
		PageTitle:   strings.Title(channelName),
		PageHours:   timeRange.String(),
		GraphParams: r.Form.Encode(),
	}

	content, err := html.Get("html/index.tmpl")
	if err != nil {
		if ioErr, ok := err.(*os.PathError); ok {
			if !os.IsNotExist(ioErr) {
				if os.IsPermission(err) {
					abort(w, err, http.StatusForbidden)
					return
				}
				abort(w, err, http.StatusInternalServerError)
				return
			}
		}
	}

	// Preform template processing
	t, err := template.New("email").Parse(string(content))
	if err != nil {
		abort(w, errors.Wrap(err, "while parsing index template"), http.StatusInternalServerError)
		return
	}

	var buf bytes.Buffer
	if err = t.Execute(&buf, data); err != nil {
		abort(w, errors.Wrap(err, "while executing index template"), http.StatusInternalServerError)
		return
	}

	if _, err := io.Copy(w, bytes.NewBuffer(buf.Bytes())); err != nil {
		abort(w, err, http.StatusInternalServerError)
	}
}

func (s *Server) redirectUI(resp http.ResponseWriter, req *http.Request) {
	s.log.Debug("Redirect to '/ui/index'")
	resp.Header().Set("Location", "/ui/index")
	resp.WriteHeader(http.StatusMovedPermanently)
}

func isValidChannel(name string, channels []SlackChannelInfo) error {
	var names []string
	for _, channel := range channels {
		names = append(names, channel.Name)
		if channel.Name == name {
			return nil
		}
	}
	return errors.Errorf("invalid 'channel' must be one of the following channel names '%s'",
		strings.Join(names, ","))
}

func filterOnlyMembers(channels []SlackChannelInfo) []SlackChannelInfo {
	var results []SlackChannelInfo
	for _, channel := range channels {
		if channel.IsMember {
			results = append(results, channel)

		}
	}
	return results
}
