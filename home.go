package main

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/n0remac/GoDom/database"
	. "github.com/n0remac/GoDom/html"
	. "github.com/n0remac/GoDom/websocket"
)

const (
	homeRoomID      = "home"
	maxUploadSizeMB = 10
)

type HomeApp struct {
	Store *database.DocumentStore
}

func Home(mux *http.ServeMux, websocketRegistry *CommandRegistry, store *database.DocumentStore) {
	registerHomeWebsocket(websocketRegistry)

	app := &HomeApp{Store: store}
	mux.HandleFunc("/", app.homeHandler())
	mux.HandleFunc("/images/upload", app.uploadImageHandler())
}

func registerHomeWebsocket(websocketRegistry *CommandRegistry) {
	websocketRegistry.RegisterWebsocket("test", func(_ string, hub *Hub, data map[string]interface{}) {
		WsHub.Broadcast <- WebsocketMessage{
			Room: homeRoomID,
			Content: []byte(
				Div(
					Id("test-message"),
					T("Received test message: "),
					Span(Class("font-bold"), T(fmt.Sprintf("%v", data["test"]))),
				).Render(),
			),
		}
	})
}

func (a *HomeApp) homeHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		images, err := a.Store.ListImages(context.Background())
		if err != nil {
			http.Error(w, "failed to load images", http.StatusInternalServerError)
			return
		}

		statusMessage := strings.TrimSpace(r.URL.Query().Get("status"))
		errorMessage := strings.TrimSpace(r.URL.Query().Get("error"))
		ServeNode(HomePage(images, statusMessage, errorMessage))(w, r)
	}
}

func (a *HomeApp) uploadImageHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		maxBytes := int64(maxUploadSizeMB) * 1024 * 1024
		r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
		if err := r.ParseMultipartForm(maxBytes); err != nil {
			http.Redirect(w, r, "/?error="+url.QueryEscape("image upload failed: file too large or invalid form"), http.StatusSeeOther)
			return
		}

		file, header, err := r.FormFile("image")
		if err != nil {
			http.Redirect(w, r, "/?error="+url.QueryEscape("please select an image file"), http.StatusSeeOther)
			return
		}
		defer file.Close()

		storedPath := buildStoredImagePath(header.Filename)
		record, err := a.Store.UploadImage(context.Background(), storedPath, file, "")
		if err != nil {
			http.Redirect(w, r, "/?error="+url.QueryEscape("failed to store image"), http.StatusSeeOther)
			return
		}
		if !strings.HasPrefix(record.ContentType, "image/") {
			_ = a.Store.DeleteImage(context.Background(), record.Path)
			http.Redirect(w, r, "/?error="+url.QueryEscape("uploaded file is not an image"), http.StatusSeeOther)
			return
		}

		http.Redirect(w, r, "/?status="+url.QueryEscape("uploaded "+record.OriginalName), http.StatusSeeOther)
	}
}

func HomePage(images []*database.ImageRecord, statusMessage, errorMessage string) *Node {
	notice := If(
		errorMessage != "",
		Div(Class("alert alert-error"), Text(errorMessage)),
		If(statusMessage != "", Div(Class("alert alert-success"), Text(statusMessage)), Nil()),
	)

	return DefaultLayout(
		Attr("hx-ext", "ws"),
		Attr("ws-connect", "/ws/hub?room="+homeRoomID),
		Div(Attrs(map[string]string{
			"class":      "flex flex-col items-center min-h-screen gap-6",
			"data-theme": "dark",
		}),
			NavBar(),
			Div(Class("w-full max-w-6xl px-4 space-y-6"),
				Div(
					Class("card p-6 bg-base-200 space-y-4"),
					H2(Text("Image Uploads")),
					P(Class("text-base-content/70"), Text("Upload an image and it will appear in the gallery below.")),
					notice,
					Form(
						Method("POST"),
						Action("/images/upload"),
						Attr("enctype", "multipart/form-data"),
						Class("flex flex-col gap-3 sm:flex-row sm:items-center"),
						Input(
							Id("image-upload"),
							Type("file"),
							Name("image"),
							Attr("accept", "image/*"),
							Attr("required", "required"),
							Class("file-input file-input-bordered w-full sm:max-w-md"),
						),
						Button(
							Type("submit"),
							Class("btn btn-primary"),
							Text("Upload"),
						),
					),
				),
				Div(
					Class("space-y-4"),
					H3(Text("Gallery")),
					imageGallery(images),
				),
			),
			Div(Id("test-message")),
			Form(
				Attr("ws-send", "submit"),
				Input(
					Type("hidden"),
					Name("type"),
					Value("test"),
				),
				Input(
					Type("hidden"),
					Name("test"),
					Value("test"),
				),
				Input(
					Type("submit"),
					Class("btn btn-primary w-32"),
					Value("Test Websocket"),
				),
			),
		),
	)
}

func imageGallery(images []*database.ImageRecord) *Node {
	if len(images) == 0 {
		return Div(
			Class("card p-6 bg-base-200 text-base-content/70"),
			Text("No images uploaded yet."),
		)
	}

	cards := make([]*Node, 0, len(images))
	for _, image := range images {
		src := "/images/" + image.Path
		cards = append(cards,
			Div(
				Class("card bg-base-200 p-3 space-y-2"),
				Div(Class("aspect-square overflow-hidden rounded-md bg-base-300"),
					Img(
						Src(src),
						Alt(image.OriginalName),
						Class("h-full w-full object-cover"),
					),
				),
				Div(Class("space-y-1"),
					P(Class("text-sm font-semibold truncate"), Text(image.OriginalName)),
					P(Class("text-xs text-base-content/60 break-all"), Text(image.Path)),
				),
			),
		)
	}

	return Div(
		Class("grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3"),
		Ch(cards),
	)
}

func buildStoredImagePath(filename string) string {
	base := filepath.Base(strings.TrimSpace(filename))
	if base == "" || base == "." {
		base = "image"
	}

	ext := strings.ToLower(filepath.Ext(base))
	name := strings.TrimSuffix(base, ext)
	slug := slugFilename(name)
	if slug == "" {
		slug = "image"
	}
	if ext == "" {
		ext = ".img"
	}

	return fmt.Sprintf("uploads/%d-%s%s", time.Now().UnixNano(), slug, ext)
}

func slugFilename(input string) string {
	input = strings.ToLower(strings.TrimSpace(input))
	if input == "" {
		return ""
	}

	var b strings.Builder
	lastDash := false
	for _, r := range input {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
			lastDash = false
		case r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		default:
			if !lastDash {
				b.WriteRune('-')
				lastDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

func NavBar() *Node {
	return Nav(Class("bg-base-300 p-4 w-full"),
		Div(Class("container mx-auto flex justify-center"),
			Ul(Class("flex space-x-6"),
				Li(A(Href("/"), T("Home"))),
				Li(A(Href("/login"), T("Login"))),
				Li(A(Href("/admin"), T("Admin"))),
				Li(A(Href("/about"), T("About"))),
			),
		),
	)
}
