package api

// Re-export response functions from the response sub-package for backward compatibility.
// New code should import github.com/jasonwa/goddi/internal/api/response directly.
import (
	"github.com/jasonwa/goddi/internal/api/response"
)

// Re-export types and functions.
type Response = response.Response
type PaginatedResponse = response.PaginatedResponse
type PageMeta = response.PageMeta

var (
	OK              = response.OK
	OKWithMessage   = response.OKWithMessage
	Created         = response.Created
	OKPaginated     = response.OKPaginated
	BadRequest      = response.BadRequest
	Unauthorized    = response.Unauthorized
	Forbidden       = response.Forbidden
	NotFound        = response.NotFound
	Conflict        = response.Conflict
	InternalError   = response.InternalError
	ParsePagination = response.ParsePagination
)
