package dashboard

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"

	"github.com/alicanalbayrak/sikifanso/internal/argocd/appsetreconcile"
	"github.com/alicanalbayrak/sikifanso/internal/catalog"
	"github.com/alicanalbayrak/sikifanso/internal/kube"
	"github.com/alicanalbayrak/sikifanso/internal/session"
	"go.uber.org/zap"
)

//go:embed templates/*.html
var templateFS embed.FS

//go:embed static/*
var staticFS embed.FS

// templateData extends ClusterData with computed fields for the template.
type templateData struct {
	*ClusterData
	EnabledCount int
	TotalApps    int
}

var tmpl *template.Template

func init() {
	funcMap := template.FuncMap{
		"healthClass": func(s string) string {
			switch s {
			case "Healthy":
				return "healthy"
			case "Degraded", "Missing":
				return "degraded"
			default:
				return "unknown"
			}
		},
		"syncClass": func(s string) string {
			switch s {
			case "Synced":
				return "synced"
			case "OutOfSync":
				return "outofsync"
			default:
				return "unknown"
			}
		},
	}
	tmpl = template.Must(template.New("").Funcs(funcMap).ParseFS(templateFS, "templates/*.html"))
}

// ServerOpts configures the dashboard server.
type ServerOpts struct {
	Addr        string
	ClusterName string
	Log         *zap.Logger
}

// NewServer creates an http.Server for the dashboard.
func NewServer(opts ServerOpts) *http.Server {
	mux := http.NewServeMux()

	staticSub, _ := fs.Sub(staticFS, "static")
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticSub))))

	mux.HandleFunc("GET /", handleIndex(opts))
	mux.HandleFunc("GET /api/status", handleStatus(opts))
	mux.HandleFunc("POST /api/catalog/{name}/toggle", handleToggle(opts))

	return &http.Server{
		Addr:    opts.Addr,
		Handler: mux,
	}
}

func handleIndex(opts ServerOpts) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}

		data, err := Gather(r.Context(), opts.ClusterName)
		if err != nil {
			opts.Log.Error("gathering cluster data", zap.Error(err))
			http.Error(w, "failed to load cluster data", http.StatusInternalServerError)
			return
		}

		td := templateData{ClusterData: data}
		for _, a := range data.CatalogApps {
			td.TotalApps++
			if a.Enabled {
				td.EnabledCount++
			}
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := tmpl.ExecuteTemplate(w, "index.html", td); err != nil {
			opts.Log.Error("rendering template", zap.Error(err))
		}
	}
}

func handleStatus(opts ServerOpts) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data, err := Gather(r.Context(), opts.ClusterName)
		if err != nil {
			opts.Log.Error("gathering cluster data", zap.Error(err))
			http.Error(w, "failed to load cluster data", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(data); err != nil {
			opts.Log.Error("encoding status response", zap.Error(err))
		}
	}
}

func handleToggle(opts ServerOpts) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		if name == "" {
			http.Error(w, "app name required", http.StatusBadRequest)
			return
		}

		sess, err := session.Load(opts.ClusterName)
		if err != nil {
			http.Error(w, "failed to load session", http.StatusInternalServerError)
			return
		}

		result, err := catalog.Flip(sess.GitOpsPath, name)
		if err != nil {
			opts.Log.Error("toggling app", zap.String("app", name), zap.Error(err))
			http.Error(w, "failed to toggle app", http.StatusInternalServerError)
			return
		}

		// Trigger ArgoCD ApplicationSet reconciliation (fire-and-forget).
		if !result.NoChange {
			ctx := context.Background()
			restCfg, restErr := kube.RESTConfigForCluster(sess.ClusterName)
			if restErr == nil {
				if reconciler, recErr := appsetreconcile.NewReconciler(restCfg, "argocd"); recErr == nil {
					if syncErr := reconciler.Trigger(ctx, "catalog"); syncErr != nil {
						opts.Log.Warn("argocd sync trigger failed", zap.Error(syncErr))
					}
				}
			}
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"name":%q,"enabled":%v}`, name, result.Enabled)
	}
}
