package register

// Request is the attributes of the POST /api/auth/register document
// ({"data": {"type": "users", "attributes": {...}}}). The password is at least
// 8 characters and at most 72 bytes — bcrypt's input limit is in bytes, and a
// character-counted max let 40 accented letters (80 bytes) through to a 500.
type Request struct {
	Name     string `json:"name" binding:"required,notblank,max=255" label:"Name"`
	Email    string `json:"email" binding:"required,email,max=255" label:"Email"`
	Password string `json:"password" binding:"required,min=8,maxbytes=72" label:"Password"`
}
