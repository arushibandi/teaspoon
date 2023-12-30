package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"html"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"tailscale.com/client/tailscale"
	"tailscale.com/tsnet"
)

const (
	imgDir  = "img"
	postDir = "post"
)

var (
	port     = flag.String("port", ":443", "port to use on the tailnet")
	hostname = flag.String("hostname", "teaspoon", "hostname to use on the tailnet")
)

type tspServer struct {
	// tailscale server
	ts *tsnet.Server
	// tailscale local client that server communicates with
	lc *tailscale.LocalClient
	// paths for read & writing
	imgPath  string
	postPath string
}

func main() {
	// Parse set flags, if any.
	flag.Parse()

	// Initialize the tailnet server.
	srv := new(tsnet.Server)
	srv.Hostname = *hostname
	srv.Store = nil
	srv.Dir = path.Join("data", "teaspoon")

	// Need the local client for some paths.
	lc, err := srv.LocalClient()
	if err != nil {
		log.Fatalf("unable to resolve local tailscale client: %s", err.Error())
		return
	}

	server := &tspServer{
		ts: srv,
		lc: lc,
		// Save images and text in different paths for convenience when browsing.
		imgPath:  (imgDir),
		postPath: (postDir),
	}

	// Set up the request handlers.
	http.Handle("/who", http.HandlerFunc(server.who))
	http.Handle("/upload", http.HandlerFunc(server.upload))
	http.Handle("/feed", http.HandlerFunc(server.feed))

	// For serving the static website
	http.Handle("/img/", http.Handler(http.StripPrefix("/img/", http.FileServer(http.Dir("img")))))
	http.Handle("/", http.Handler(http.FileServer(http.Dir("web"))))

	// Start running the server
	ln, err := srv.ListenTLS("tcp", *port)
	if err != nil {
		log.Fatalf("unable to start listening on the tailscale server: %s", err.Error())
		return
	}
	defer ln.Close()
	log.Fatal(http.Serve(ln, nil))
}

// who returns a simple HTML page stating the identity of the viewer in the tailnet.
// https://github.com/tailscale/tailscale/blob/main/tsnet/example/tshello/tshello.go
func (s *tspServer) who(w http.ResponseWriter, r *http.Request) {
	who, err := s.lc.WhoIs(r.Context(), r.RemoteAddr)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	fmt.Fprintf(w, "<a href=\"/feed.html\">↩️ back to feed</a>")
	fmt.Fprintf(w, "<html><body><h1>Hello, tailnet!</h1>\n")
	fmt.Fprintf(w, "<p>You are <b>%s</b> from <b>%s</b> (%s)</p>",
		html.EscapeString(who.UserProfile.LoginName),
		html.EscapeString(who.Node.ComputedName),
		r.RemoteAddr)
}

func (s *tspServer) writeFile(file multipart.File, header *multipart.FileHeader, w http.ResponseWriter) (string, error) {
	defer file.Close()
	// Create a new file in the img directory
	imgPath := fmt.Sprintf(path.Join(s.imgPath, "%d_%s"), time.Now().UnixNano(), filepath.Ext(header.Filename))
	dst, err := os.Create(imgPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		log.Print("3", err.Error())
		return "", nil
	}
	defer dst.Close()

	// Copy the uploaded file to the filesystem
	// at the specified destination
	_, err = io.Copy(dst, file)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		log.Print("4", err.Error())
		return "", err
	}
	return imgPath, nil
}

type uploadRequest struct {
	Note string `json:"Note"`
}

type post struct {
	ID      string `json:"ID"`
	Note    string `json:"Note"`
	Author  string `json:"Author"`
	ImgPath string `json:"Img"`
}

// post allows a user to add a post to the website
func (s *tspServer) upload(w http.ResponseWriter, r *http.Request) {
	// Ensure POST request.
	if r.Method != http.MethodPost {
		http.Error(w, "only POST requests are supported at this endpoint", http.StatusMethodNotAllowed)
		return
	}

	postData := r.FormValue("post")

	// Read request body.
	slurp, err := io.ReadAll(strings.NewReader(postData))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		log.Print("0", err.Error())
		return
	}

	// Parse post as structured data.
	var req uploadRequest
	err = json.Unmarshal(slurp, &req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		log.Print("1", err.Error())
		return
	}

	// Get uploaded file, if any.
	file, header, err := r.FormFile("img")
	var imgPath string
	if errors.Is(err, http.ErrMissingFile) {
		log.Print("no file uploaded with post")
	} else if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		log.Print("2", err.Error())
		return
	} else {
		imgPath, err = s.writeFile(file, header, w)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			log.Print("write file errr ", err.Error())
			return
		}
	}

	id := uuid.NewString()
	// Set the "author" property to the machine name.
	who, err := s.lc.WhoIs(r.Context(), r.RemoteAddr)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	// TODO(@arushibandi): do some validation of data?s
	p := &post{
		ID:     id,
		Note:   req.Note,
		Author: who.Node.ComputedName,
	}
	if imgPath != "" {
		p.ImgPath = imgPath
	}

	// Write post to local db.
	bytes, err := json.Marshal(p)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		log.Print("6", err.Error())
		return
	}

	fmt.Println("Writning", string(bytes))

	err = os.WriteFile(path.Join(s.postPath, fmt.Sprintf("%s.json", id)), bytes, os.ModePerm)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		log.Print("7", err.Error())
		return
	}
}

type feedResponse struct {
	Posts []json.RawMessage `json:"Posts"`
}

// postIds returns the IDs of all posts saved on the server.
func (s *tspServer) feed(w http.ResponseWriter, r *http.Request) {
	// Ensure GET request.
	if r.Method != http.MethodGet {
		http.Error(w, "only GET requests are supported at this endpoint", http.StatusMethodNotAllowed)
		return
	}

	// Get all postIDs.
	posts, err := os.ReadDir(s.postPath)
	if err != nil {
		http.Error(w, fmt.Sprintf("error getting new post id %s", err.Error()), http.StatusInternalServerError)
		log.Print("0", err.Error())
		return
	}

	sort.Slice(posts, func(i, j int) bool {
		infoI, err := posts[i].Info()
		if err != nil {
			return true
		}

		infoJ, err := posts[j].Info()
		if err != nil {
			return true
		}

		return infoI.ModTime().After(infoJ.ModTime())
	})

	feed := make([]json.RawMessage, len(posts))
	for i, p := range posts {
		bytes, err := os.ReadFile(path.Join(s.postPath, p.Name()))
		if err != nil {
			http.Error(w, fmt.Sprintf("error reading contents of post JSON file %s", err.Error()), http.StatusInternalServerError)
			log.Print("1", err.Error())
			return
		}
		feed[i] = bytes
	}

	bytes, err := json.Marshal(feedResponse{
		Posts: feed,
	})
	if err != nil {
		http.Error(w, fmt.Sprintf("error marshalling post IDs response %s", err.Error()), http.StatusInternalServerError)
		log.Print("2", err.Error())
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Write(bytes)
}
