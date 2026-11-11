package passwordresetrequest

// ResourceType is the JSON:API type of the request document. It has no id.
const ResourceType = "password-reset-requests"

// Request is the attributes of the POST /api/auth/password-reset-requests
// document.
type Request struct {
	Email string `json:"email" binding:"required,email,max=255" label:"Email"`
}
