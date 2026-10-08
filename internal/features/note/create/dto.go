package create

// Request is the attributes of the POST /api/notes document
// ({"data": {"type": "notes", "attributes": {...}}}). The `label` tag is the
// field name shown in a 422 detail — keep it in the client form's wording.
type Request struct {
	Title string `json:"title" binding:"required,notblank,max=255" label:"Title"`
	Body  string `json:"body" binding:"max=10000" label:"Content"`
}
