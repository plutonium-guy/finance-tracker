package web

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"finance-tracker/internal/domain"
	"finance-tracker/internal/store"
)

type settingsVM struct {
	base
	Categories []domain.Category
}

type catListVM struct {
	Categories []domain.Category
}

func (h *Handler) SettingsPage(w http.ResponseWriter, r *http.Request) {
	cats, _ := h.svc.Store.ListCategories()
	h.rdr.Page(w, "settings", settingsVM{base: h.base("Settings", "settings"), Categories: cats})
}

func (h *Handler) SettingsSave(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	s, _ := h.svc.Store.GetSettings()
	if fy := r.FormValue("fiscal_year_start"); fy == "April" || fy == "January" {
		s.FiscalYearStart = fy
	}
	s.DarkMode = r.FormValue("dark_mode") != ""
	if err := h.svc.Store.SaveSettings(s); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	h.flash(w, "Preferences saved")
}

func (h *Handler) CategoriesList(w http.ResponseWriter, r *http.Request) {
	cats, _ := h.svc.Store.ListCategories()
	h.rdr.Fragment(w, "category_list", catListVM{Categories: cats})
}

func (h *Handler) CategoryAdd(w http.ResponseWriter, r *http.Request) {
	name := r.FormValue("name")
	if name != "" {
		h.svc.Store.AddCategory(name)
	}
	h.CategoriesList(w, r)
}

func (h *Handler) CategoryRename(w http.ResponseWriter, r *http.Request) {
	old := chi.URLParam(r, "name")
	// htmx sends the prompt value in the HX-Prompt header; fall back to a form field.
	newName := r.Header.Get("HX-Prompt")
	if newName == "" {
		newName = r.FormValue("new_name")
	}
	if newName != "" {
		h.svc.Store.RenameCategory(old, newName)
	}
	h.CategoriesList(w, r)
}

func (h *Handler) CategoryDelete(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	reassign := r.FormValue("reassign")
	if err := h.svc.Store.DeleteCategory(name, reassign); err == store.ErrCategoryInUse {
		w.Header().Set("HX-Retarget", "#flash")
		h.flash(w, "Category \""+name+"\" is in use — choose a category to reassign first")
		return
	}
	h.CategoriesList(w, r)
}

func (h *Handler) Backup(w http.ResponseWriter, r *http.Request) {
	b, err := h.svc.Store.Export()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", "attachment; filename=finance-backup.json")
	json.NewEncoder(w).Encode(b)
}

func (h *Handler) Restore(w http.ResponseWriter, r *http.Request) {
	file, _, err := r.FormFile("file")
	if err != nil {
		h.flash(w, "No file uploaded")
		return
	}
	defer file.Close()
	var b store.Backup
	if err := json.NewDecoder(file).Decode(&b); err != nil {
		h.flash(w, "Invalid backup JSON")
		return
	}
	if err := h.svc.Store.Import(b); err != nil {
		h.flash(w, "Restore failed: "+err.Error())
		return
	}
	txTrigger(w)
	h.flash(w, "Restored from backup")
}

func (h *Handler) Reset(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.Store.Reset(); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	txTrigger(w)
	h.flash(w, "Data reset to defaults")
}
