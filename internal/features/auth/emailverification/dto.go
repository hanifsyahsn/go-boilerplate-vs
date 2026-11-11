package emailverification

// ResourceType is the JSON:API type of the request document. It has no id.
const ResourceType = "email-verifications"

// Request is the attributes of the POST /api/auth/email-verifications
// document: the link's token.
type Request struct {
	Token string `json:"token" binding:"required,max=64" label:"Token"`
}
