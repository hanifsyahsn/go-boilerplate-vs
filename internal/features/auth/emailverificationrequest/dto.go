package emailverificationrequest

// ResourceType is the JSON:API type of the (optional) request document.
const ResourceType = "email-verification-requests"

// Request is the request document's attributes: none. A body is optional;
// {"data":{"type":"email-verification-requests"}} is accepted.
type Request struct{}
