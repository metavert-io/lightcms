package handlers

import (
	"net/http"
	"time"

	"github.com/metavert-io/lightcms/v7/internal/auth"

	"github.com/gorilla/mux"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// POST /api/v1/content/{id}/schedule — set or clear publish_at
func (a *APIHandler) APIScheduleContentPublish(w http.ResponseWriter, r *http.Request) {
	if !a.requirePermission(w, r, auth.PermContentPublish) {
		return
	}

	contentID, err := primitive.ObjectIDFromHex(mux.Vars(r)["id"])
	if err != nil {
		a.jsonError(w, http.StatusBadRequest, "invalid content id")
		return
	}

	var req struct {
		PublishAt *string `json:"publish_at"` // ISO 8601; null clears the schedule
	}
	if err := a.decodeJSON(r, &req); err != nil {
		a.jsonError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	var publishAt *time.Time
	if req.PublishAt != nil && *req.PublishAt != "" {
		t, err := time.Parse(time.RFC3339, *req.PublishAt)
		if err != nil {
			a.jsonError(w, http.StatusBadRequest, "publish_at must be ISO 8601 (e.g. 2026-03-24T15:00:00Z)")
			return
		}
		publishAt = &t
	}

	// A fork copy goes live only through its fork's merge, never on a schedule.
	if content, err := a.contentService.GetContent(r.Context(), contentID); err == nil && content.ForkID != nil {
		a.jsonError(w, http.StatusBadRequest, "a fork copy can't be scheduled; it goes live when its fork is merged")
		return
	}

	update := bson.M{"$set": bson.M{"updated_at": time.Now()}}
	if publishAt != nil {
		update["$set"].(bson.M)["publish_at"] = *publishAt
	} else {
		update["$unset"] = bson.M{"publish_at": ""}
	}

	err = a.contentService.DB().UpdateOne(r.Context(), "content",
		bson.M{"_id": contentID, "deleted": bson.M{"$ne": true}}, update)
	if err != nil {
		a.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if publishAt != nil {
		a.jsonResponse(w, http.StatusOK, map[string]interface{}{
			"content_id": contentID.Hex(),
			"publish_at": publishAt.Format(time.RFC3339),
			"message":    "publish scheduled",
		})
	} else {
		a.jsonResponse(w, http.StatusOK, map[string]interface{}{
			"content_id": contentID.Hex(),
			"publish_at": nil,
			"message":    "scheduled publish cleared",
		})
	}
}

// GET /api/v1/content/scheduled — list content with publish_at set and not yet published
func (a *APIHandler) APIListScheduledContent(w http.ResponseWriter, r *http.Request) {
	if !a.requirePermission(w, r, auth.PermContentView) {
		return
	}

	filter := bson.M{
		"publish_at": bson.M{"$ne": nil, "$exists": true},
		"published":  false,
		"deleted":    bson.M{"$ne": true},
	}

	folder := r.URL.Query().Get("folder")
	if folder != "" {
		filter["folder_path"] = folder
	}

	opts := options.Find().
		SetSort(bson.D{{Key: "publish_at", Value: 1}}).
		SetLimit(200)

	cursor, err := a.contentService.DB().FindMany(r.Context(), "content", filter, opts)
	if err != nil {
		a.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer cursor.Close(r.Context())

	type scheduledItem struct {
		ID        string `json:"id"`
		Title     string `json:"title"`
		FullPath  string `json:"full_path"`
		PublishAt string `json:"publish_at"`
	}

	var results []scheduledItem
	for cursor.Next(r.Context()) {
		var item struct {
			ID        primitive.ObjectID `bson:"_id"`
			Title     string             `bson:"title"`
			FullPath  string             `bson:"full_path"`
			PublishAt *time.Time         `bson:"publish_at,omitempty"`
		}
		if err := cursor.Decode(&item); err != nil {
			continue
		}
		pa := ""
		if item.PublishAt != nil {
			pa = item.PublishAt.Format(time.RFC3339)
		}
		results = append(results, scheduledItem{
			ID:        item.ID.Hex(),
			Title:     item.Title,
			FullPath:  item.FullPath,
			PublishAt: pa,
		})
	}
	if results == nil {
		results = []scheduledItem{}
	}
	a.jsonResponse(w, http.StatusOK, results)
}
