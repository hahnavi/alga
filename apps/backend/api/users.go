package api

import (
	"errors"
	"net/http"
	"strings"

	algacrypto "alga/crypto"
	"alga/logger"
	"alga/rbac"
	"alga/store"

	"github.com/google/uuid"
)

func (s *Server) handleUsers(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		users, err := s.userStore.ListUsers()
		if err != nil {
			writeInternalError(w, err, "failed to list users")
			return
		}
		writeData(w, http.StatusOK, users)

	case http.MethodPost:
		var req struct {
			Email    string `json:"email"`
			Password string `json:"password"`
			Role     string `json:"role"`
			FullName string `json:"full_name,omitempty"`
		}
		if !decodeJSON(w, r, &req) {
			return
		}
		if req.Email == "" || req.Password == "" {
			writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "email and password are required")
			return
		}
		if !rbac.ValidRole(req.Role) {
			writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "role must be 'admin', 'operator', or 'viewer'")
			return
		}

		if err := validatePasswordPolicy(req.Password); err != nil {
			writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, err.Error())
			return
		}

		record, err := s.userStore.CreateUser(req.Email, req.Password, req.Role)
		if err != nil {
			writeInternalError(w, err, "failed to create user")
			return
		}

		updates := map[string]any{}
		if req.FullName != "" {
			updates["full_name"] = req.FullName
		}
		if len(updates) > 0 {
			if err := s.userStore.UpdateUser(record.ID, updates); err != nil {
				logger.ErrorCtx(r.Context(), "failed to set user profile", "component", "api", "user_id", record.ID.String(), "error", err)
				writeInternalError(w, err, "failed to set user profile")
				return
			}
			record.FullName = req.FullName
		}

		logger.InfoCtx(r.Context(), "user created", "component", "api", "user_id", record.ID.String(), "email", record.Email)
		s.audit(r, store.AuditUserCreated, map[string]any{
			"user_id": record.ID.String(),
			"email":   record.Email,
			"role":    record.Role,
		})
		writeData(w, http.StatusCreated, record)

	default:
		writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "method not allowed")
	}
}

func (s *Server) handleUserByID(w http.ResponseWriter, r *http.Request) {
	idHex := pathID(r, "/api/v1/users/")
	id, err := uuid.Parse(idHex)
	if err != nil {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "invalid user id")
		return
	}

	switch r.Method {
	case http.MethodPut:
		var req struct {
			Role         string  `json:"role,omitempty"`
			Password     string  `json:"password,omitempty"`
			Email        *string `json:"email,omitempty"`
			FullName     *string `json:"full_name,omitempty"`
			Phone        *string `json:"phone,omitempty"`
			PhoneCountry *string `json:"phone_country,omitempty"`
			VoiceOptOut  *bool   `json:"voice_opt_out,omitempty"`
		}
		if !decodeJSON(w, r, &req) {
			return
		}

		updates := map[string]any{}
		if req.Role != "" {
			if !rbac.ValidRole(req.Role) {
				writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "role must be 'admin', 'operator', or 'viewer'")
				return
			}
			if req.Role != "admin" {
				target, err := s.userStore.GetByID(id)
				if err != nil || target == nil {
					writeError(w, ErrorCodeNotFound, "user not found")
					return
				}
				if target.Role == "admin" {
					adminCount, acErr := s.userStore.CountAdmins()
					if acErr != nil {
						writeInternalError(w, acErr, "failed to count admins")
						return
					}
					if adminCount <= 1 {
						writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "cannot demote the last admin")
						return
					}
				}
			}
			updates["role"] = req.Role
		}
		if req.Password != "" {
			if err := validatePasswordPolicy(req.Password); err != nil {
				writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, err.Error())
				return
			}
			updates["password"] = req.Password // will be hashed in UpdateUser
		}
		if req.Email != nil {
			updates["email"] = *req.Email
		}
		if req.FullName != nil {
			updates["full_name"] = *req.FullName
		}
		if req.Phone != nil {
			normalized, err := validatePhoneNumber(*req.Phone)
			if err != nil {
				writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, err.Error())
				return
			}
			updates["phone"] = normalized
		}
		if req.PhoneCountry != nil {
			phoneForCheck := ""
			if p, ok := updates["phone"].(string); ok {
				phoneForCheck = p
			}
			if phoneForCheck == "" && req.Phone == nil {
				if cur, err := s.userStore.GetByID(id); err == nil && cur != nil {
					phoneForCheck = cur.Phone
				}
			}
			resolved, err := validatePhoneCountry(strings.ToUpper(strings.TrimSpace(*req.PhoneCountry)), phoneForCheck)
			if err != nil {
				writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, err.Error())
				return
			}
			updates["phone_country"] = resolved
		}
		if req.VoiceOptOut != nil {
			updates["voice_opt_out"] = *req.VoiceOptOut
		}

		if len(updates) == 0 {
			writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "no fields to update")
			return
		}

		// Hash password if provided
		if pw, ok := updates["password"]; ok {
			hash, err := hashPassword(pw.(string))
			if err != nil {
				writeError(w, ErrorCodeInternal, "failed to hash password")
				return
			}
			updates["password"] = hash
		}

		if err := s.userStore.UpdateUser(id, updates); err != nil {
			logger.ErrorCtx(r.Context(), "failed to update user", "component", "api", "user_id", id.String(), "error", err)
			writeInternalError(w, err, "failed to update user")
			return
		}
		logger.InfoCtx(r.Context(), "user updated", "component", "api", "user_id", id.String())
		s.audit(r, store.AuditUserUpdated, map[string]any{
			"user_id": id.String(),
		})
		writeStatus(w, "updated")

	case http.MethodDelete:
		currentUser := userFromContext(r.Context())
		if currentUser != nil && currentUser.ID == id {
			writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "cannot delete yourself")
			return
		}

		target, err := s.userStore.GetByID(id)
		if err != nil || target == nil {
			writeError(w, ErrorCodeNotFound, "user not found")
			return
		}

		if target.Role == "admin" {
			adminCount, err := s.userStore.CountAdmins()
			if err != nil {
				writeInternalError(w, err, "failed to count admins")
				return
			}
			if adminCount <= 1 {
				writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "cannot delete the last admin")
				return
			}
		}

		if err := s.userStore.DeleteUser(id); err != nil {
			if errors.Is(err, store.ErrUserNotFound) {
				writeError(w, ErrorCodeNotFound, "user not found")
				return
			}
			logger.ErrorCtx(r.Context(), "failed to delete user", "component", "api", "user_id", id.String(), "error", err)
			writeInternalError(w, err, "failed to delete user")
			return
		}
		logger.InfoCtx(r.Context(), "user deleted", "component", "api", "user_id", id.String())
		s.audit(r, store.AuditUserDeleted, map[string]any{
			"user_id": id.String(),
		})
		writeStatus(w, "deleted")

	default:
		writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "method not allowed")
	}
}

// hashPassword wraps the central crypto.HashPassword so call sites in the API
// package keep a familiar one-arg signature.
func hashPassword(password string) (string, error) {
	return algacrypto.HashPassword(password)
}

// Ensure store.UserRecord is used (compile check)
var _ *store.UserRecord = nil
