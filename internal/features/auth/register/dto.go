package register

// Request is the attributes of the POST /api/auth/register document
// ({"data": {"type": "users", "attributes": {...}}}). The password is capped
// at 72 characters, bcrypt's input limit.
type Request struct {
	Name     string `json:"name" binding:"required,notblank,max=255" label:"Name"`
	Email    string `json:"email" binding:"required,email,max=255" label:"Email"`
	Password string `json:"password" binding:"required,min=8,max=72" label:"Password"`
}
