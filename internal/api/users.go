package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/pagefire/pagefire/internal/auth"
	"github.com/pagefire/pagefire/internal/store"
)

type UserHandler struct {
	users store.UserStore
}

func NewUserHandler(users store.UserStore) *UserHandler {
	return &UserHandler{users: users}
}

func (h *UserHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Post("/", h.create)
	r.Get("/", h.list)
	r.Get("/{id}", h.get)
	r.Put("/{id}", h.update)
	r.Delete("/{id}", h.delete)

	return r
}

func (h *UserHandler) create(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name     string `json:"name"`
		Email    string `json:"email"`
		Timezone string `json:"timezone"`
		Role     string `json:"role"`
		Password string `json:"password"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" || req.Email == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "username and password are required")
		return
	}
	if err := validatePassword(req.Password); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	role := store.RoleUser
	if req.Role == store.RoleAdmin {
		caller := UserFromContext(r.Context())
		if caller != nil && caller.Role == store.RoleAdmin {
			role = store.RoleAdmin
		}
	}

	tz := req.Timezone
	if tz == "" {
		tz = "UTC"
	}
	if !validateTimezone(tz) {
		writeError(w, http.StatusBadRequest, "invalid timezone")
		return
	}

	passwordHash, err := auth.HashPassword(req.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	u := &store.User{
		Name:         req.Name,
		Email:        req.Email,
		Role:         role,
		Timezone:     tz,
		PasswordHash: passwordHash,
		IsActive:     true,
	}
	if err := h.users.Create(r.Context(), u); err != nil {
		handleStoreError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"id":       u.ID,
		"name":     u.Name,
		"email":    u.Email,
		"role":     u.Role,
		"timezone": u.Timezone,
	})
}

func (h *UserHandler) get(w http.ResponseWriter, r *http.Request) {
	u, err := h.users.Get(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		handleStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, u)
}

func (h *UserHandler) list(w http.ResponseWriter, r *http.Request) {
	users, err := h.users.List(r.Context())
	if err != nil {
		handleStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, users)
}

func (h *UserHandler) update(w http.ResponseWriter, r *http.Request) {
	var u store.User
	if err := decodeJSON(w, r, &u); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	u.ID = chi.URLParam(r, "id")
	if u.Timezone != "" && !validateTimezone(u.Timezone) {
		writeError(w, http.StatusBadRequest, "invalid timezone")
		return
	}
	// Preserve existing role — role changes not allowed via this endpoint
	existing, err := h.users.Get(r.Context(), u.ID)
	if err != nil {
		handleStoreError(w, err)
		return
	}
	u.Role = existing.Role
	if err := h.users.Update(r.Context(), &u); err != nil {
		handleStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, u)
}

func (h *UserHandler) delete(w http.ResponseWriter, r *http.Request) {
	targetID := chi.URLParam(r, "id")

	// Prevent self-deletion
	caller := UserFromContext(r.Context())
	if caller != nil && caller.ID == targetID {
		writeError(w, http.StatusBadRequest, "cannot delete your own account")
		return
	}

	if err := h.users.Delete(r.Context(), targetID); err != nil {
		handleStoreError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
