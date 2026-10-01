package api

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/pagefire/pagefire/internal/homealerts"
)

type HomeAlertHandler struct{ service *homealerts.Service }

func NewHomeAlertHandler(service *homealerts.Service) *HomeAlertHandler {
	return &HomeAlertHandler{service: service}
}

func (h *HomeAlertHandler) CreateEvent(w http.ResponseWriter, r *http.Request) {
	var req homealerts.EventRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON request")
		return
	}
	result, err := h.service.Process(r.Context(), UserFromContext(r.Context()).ID, req)
	if err != nil {
		homeError(w, err)
		return
	}
	status := http.StatusCreated
	if result.Status != "applied" {
		status = http.StatusOK
	}
	writeJSON(w, status, result)
}

func (h *HomeAlertHandler) GetIngestionKey(w http.ResponseWriter, r *http.Request) {
	info, err := h.service.IngestionKey(r.Context())
	if err != nil {
		handleStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, info)
}

func (h *HomeAlertHandler) RotateIngestionKey(w http.ResponseWriter, r *http.Request) {
	token, err := h.service.RotateIngestionKey(r.Context(), UserFromContext(r.Context()).ID)
	if err != nil {
		handleStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"token": token})
}

func (h *HomeAlertHandler) RememberIngestionKey(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Token string `json:"token"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON request")
		return
	}
	if err := h.service.RememberIngestionKey(r.Context(), req.Token); err != nil {
		homeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *HomeAlertHandler) RevealIngestionKey(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	token, err := h.service.RevealIngestionKey(r.Context())
	if err != nil {
		homeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"token": token})
}

func (h *HomeAlertHandler) RevokeIngestionKey(w http.ResponseWriter, r *http.Request) {
	if err := h.service.RevokeIngestionKey(r.Context()); err != nil {
		handleStoreError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *HomeAlertHandler) GetEvent(w http.ResponseWriter, r *http.Request) {
	detail, err := h.service.GetEvent(r.Context(), UserFromContext(r.Context()).ID, chi.URLParam(r, "eventID"))
	if err != nil {
		homeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, detail)
}

func (h *HomeAlertHandler) ListEvents(w http.ResponseWriter, r *http.Request) {
	items, err := h.service.ListEvents(r.Context(), UserFromContext(r.Context()).ID)
	if err != nil {
		homeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *HomeAlertHandler) ListActiveAlerts(w http.ResponseWriter, r *http.Request) {
	items, err := h.service.ListActiveAlerts(r.Context(), UserFromContext(r.Context()).ID)
	if err != nil {
		homeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *HomeAlertHandler) GetActiveAlert(w http.ResponseWriter, r *http.Request) {
	item, err := h.service.GetActiveAlert(r.Context(), UserFromContext(r.Context()).ID, chi.URLParam(r, "alertID"))
	if err != nil {
		homeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (h *HomeAlertHandler) ListAlertEvents(w http.ResponseWriter, r *http.Request) {
	items, err := h.service.AlertEvents(r.Context(), UserFromContext(r.Context()).ID, chi.URLParam(r, "alertID"))
	if err != nil {
		homeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *HomeAlertHandler) ForceCloseAlert(w http.ResponseWriter, r *http.Request) {
	result, err := h.service.ForceClose(r.Context(), UserFromContext(r.Context()).ID, chi.URLParam(r, "alertID"))
	if err != nil {
		homeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *HomeAlertHandler) Feed(w http.ResponseWriter, r *http.Request) {
	limit, offset := parseListLimit(r)
	filter := homealerts.FeedFilter{
		Window: r.URL.Query().Get("window"), Type: r.URL.Query().Get("type"),
		Class: r.URL.Query().Get("class"), Source: r.URL.Query().Get("source"),
		Client: r.URL.Query().Get("client"), MessageRegex: r.URL.Query().Get("message_regex"),
		Limit: limit, Offset: offset,
	}
	if filter.Window == "" {
		filter.Window = "1d"
	}
	if open := r.URL.Query().Get("open"); open != "" {
		value, err := strconv.ParseBool(open)
		if err != nil {
			writeError(w, http.StatusBadRequest, "open must be true or false")
			return
		}
		filter.Open = &value
	}
	feed, err := h.service.Feed(r.Context(), UserFromContext(r.Context()).ID, filter)
	if err != nil {
		homeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, feed)
}

func (h *HomeAlertHandler) Stats(w http.ResponseWriter, r *http.Request) {
	to := time.Now().UTC().Unix() + 1
	from := to - 7*24*3600
	if v := r.URL.Query().Get("from"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, "from must be Unix seconds")
			return
		}
		from = n
	}
	if v := r.URL.Query().Get("to"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, "to must be Unix seconds")
			return
		}
		to = n
	}
	if from >= to {
		writeError(w, http.StatusBadRequest, "from must be earlier than to")
		return
	}
	stats, err := h.service.Stats(r.Context(), from, to)
	if err != nil {
		homeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

func (h *HomeAlertHandler) Plugins(w http.ResponseWriter, r *http.Request) {
	items, err := h.service.Plugins(r.Context())
	if err != nil {
		homeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *HomeAlertHandler) PutPlugin(w http.ResponseWriter, r *http.Request) {
	var input homealerts.PluginInput
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON request")
		return
	}
	plugin, err := h.service.PutPlugin(r.Context(), chi.URLParam(r, "kind"), input)
	if err != nil {
		homeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, plugin)
}

func (h *HomeAlertHandler) DeletePlugin(w http.ResponseWriter, r *http.Request) {
	if err := h.service.DeletePlugin(r.Context(), chi.URLParam(r, "kind")); err != nil {
		homeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *HomeAlertHandler) TestPlugin(w http.ResponseWriter, r *http.Request) {
	if err := h.service.TestPlugin(r.Context(), chi.URLParam(r, "kind")); err != nil {
		if errors.Is(err, homealerts.ErrNotFound) {
			homeError(w, err)
		} else {
			writeError(w, http.StatusBadGateway, err.Error())
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "sent"})
}

func homeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, homealerts.ErrInvalid):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, homealerts.ErrNotFound):
		writeError(w, http.StatusNotFound, "home alert not found")
	default:
		handleStoreError(w, err)
	}
}
