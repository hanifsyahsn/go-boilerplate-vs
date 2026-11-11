package passwordreset

// ResourceType is the JSON:API type of the request document. It has no id.
const ResourceType = "password-resets"

// Request is the attributes of the POST /api/auth/password-resets document:
// the link's token and the new password (8 characters to 72 bytes, bcrypt's
// limit).
type Request struct {
	Token    string `json:"token" binding:"required,max=64" label:"Token"`
	Password string `json:"password" binding:"required,min=8,maxbytes=72" label:"Password"`
}
