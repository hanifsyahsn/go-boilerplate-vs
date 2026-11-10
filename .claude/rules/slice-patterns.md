---
paths:
  - "internal/features/**"
  - "internal/platform/db/migration/**"
  - "internal/platform/db/tx/**"
  - "internal/shared/testhelper/**"
  - "sqlc.yaml"
---
# Slice patterns (reference implementations)

How a slice does the recurring things, written down so the knowledge **does not depend on any slice
existing**. The code is the `notes` example domain as it was in `go_boilerplate_vs` (2026-11-04) — copy it and
rename `note` → your resource. Related rules: jsonapi.md (request documents, query parameters, responses),
logging.md, auth.md. The slice layout and the adding-a-slice checklist are in the root CLAUDE.md.

**Maintenance rule.** A pattern is never removed from this file because no slice uses it right now. When a
real slice uses a pattern, add it to that pattern's `Used by:` line; when the slice is deleted, remove only
that line entry. A new recurring pattern gets a section here in the same change that introduces it.

## 1. An owned resource: table + `<domain>/shared`

Used by: `internal/features/note/*`.

Migration — owner column, soft delete, partial index for the list, the `updated_at` trigger:

```sql
CREATE TABLE notes
(
    id         bigserial PRIMARY KEY,
    title      varchar(255) NOT NULL,
    body       text         NOT NULL DEFAULT '',
    created_by bigint       NOT NULL REFERENCES users (id),
    created_at timestamptz  NOT NULL DEFAULT now(),
    updated_at timestamptz  NOT NULL DEFAULT now(),
    deleted_at timestamptz,
    deleted_by bigint REFERENCES users (id)
);
-- The list reads a creator's live notes, newest first.
CREATE INDEX notes_created_by_live_idx ON notes (created_by, created_at DESC) WHERE deleted_at IS NULL;
CREATE TRIGGER notes_updated_at BEFORE UPDATE ON notes FOR EACH ROW EXECUTE PROCEDURE update_updated_at_column();
```

`<domain>/shared` holds what ≥ 2 slices need, in four files:

- `resource.go` — the resource type const, the include policy, the response struct(s), a `Fields` struct
  every slice's sqlc row maps onto (rows are distinct generated types with the same columns), the builder:

```go
const NotesRT = "notes"

var NoteIncludes = jsonapi.IncludeConfig{Allowed: []string{"created_by"}, Default: []string{"created_by"}}

type NoteResource struct {
	ID        string       `json:"id" jsonapi:"id"`
	Title     string       `json:"title"`
	Body      string       `json:"body"`
	CreatedAt time.Time    `json:"created_at"`
	UpdatedAt time.Time    `json:"updated_at"`
	CreatedBy UserResource `jsonapi:"relation,created_by,users"`
}
type UserResource struct {
	ID    string `json:"id" jsonapi:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}
type NoteFields struct { ID int64; Title, Body string; CreatedBy int64; CreatedAt, UpdatedAt time.Time; CreatorName, CreatorEmail string }

func NewNoteResource(f NoteFields) NoteResource // ids → strconv.FormatInt, creator → UserResource
```

- `query.go` — the fieldsets map and the `QuerySpec` of every single-resource endpoint:

```go
const UsersRT = "users"
var NoteFieldsets = map[string]any{NotesRT: NoteResource{}, UsersRT: UserResource{}}
var NoteQuery = jsonapi.QuerySpec{Include: &NoteIncludes, Fields: NoteFieldsets}
```

- `query/queries.sql` — the read (with the to-one JOIN) and the lock; nothing else unless two slices need it:

```sql
-- name: GetNoteByID :one
SELECT n.id, n.title, n.body, n.created_by, n.created_at, n.updated_at,
       u.name AS creator_name, u.email AS creator_email
FROM notes n JOIN users u ON u.id = n.created_by
WHERE n.id = $1 AND n.deleted_at IS NULL;

-- name: LockNoteForUpdate :one
-- Locks a live row for the rest of the transaction; a concurrent update / delete waits.
SELECT id, created_by FROM notes WHERE id = $1 AND deleted_at IS NULL FOR UPDATE;
```

- `access.go` — **ownership: missing, deleted and someone else's all read the same 404** (existence never
  leaks to a non-owner):

```go
const NotFoundMessage = "We couldn't find that note. It may have been removed."

func GetOwnedNote(ctx context.Context, q sqlc.Querier, id, userID int64, feature string) (sqlc.GetNoteByIDRow, error) {
	note, err := q.GetNoteByID(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return sqlc.GetNoteByIDRow{}, apperror.New(apperror.CodeNotFound, NotFoundMessage,
			fmt.Errorf("GetNoteByID(%d) (%s): %w", id, feature, err))
	}
	if err != nil {
		return sqlc.GetNoteByIDRow{}, apperror.New(apperror.CodeInternal,
			"We couldn't load the note. Please try again, and contact support if the problem continues.",
			fmt.Errorf("GetNoteByID(%d) (%s): %w", id, feature, err))
	}
	if err = EnsureOwner(note.CreatedBy, userID, id, feature); err != nil {
		return sqlc.GetNoteByIDRow{}, err
	}
	return note, nil
}

// LockOwnedNote: the same, over q.LockNoteForUpdate, returning only the error.

func EnsureOwner(createdBy, userID, noteID int64, feature string) error {
	if createdBy != userID {
		return apperror.New(apperror.CodeNotFound, NotFoundMessage,
			fmt.Errorf("note %d not owned by caller %d (%s)", noteID, userID, feature))
	}
	return nil
}

func FieldsFromRow(r sqlc.GetNoteByIDRow) NoteFields
```

Visibility wider than "owner only" (an approver, an admin, a role) changes `EnsureOwner`, never the rule that
"can't see it" reads as 404.

## 2. Create: insert, then read back through the shared query

Used by: `note/create`.

No transaction (one insert); the service takes the slice's querier and the shared one. Reading back makes the
response carry the JOINed relationship and the DB-set timestamps, identical to detail's.

```go
func NewService(noteSharedQuerier notesharedsqlc.Querier, querier sqlc.Querier) S

func (s *service) Create(ctx context.Context, req Request, userID int64) (shared.NoteResource, error) {
	id, err := s.querier.CreateNote(ctx, sqlc.CreateNoteParams{Title: strings.TrimSpace(req.Title), Body: req.Body, CreatedBy: userID})
	if err != nil {
		return shared.NoteResource{}, apperror.New(apperror.CodeInternal,
			"We couldn't save the note. Please try again, and contact support if the problem continues.",
			fmt.Errorf("querier.CreateNote() (%s): %w", feature, err))
	}
	note, err := shared.GetOwnedNote(ctx, s.noteSharedQuerier, id, userID, feature)
	if err != nil {
		return shared.NoteResource{}, err
	}
	return shared.NewNoteResource(shared.FieldsFromRow(note)), nil
}
```

Handler: no `ResourceSpec.ID` (a client id is a 403), `201` + `Location`:

```go
q, ok := handlerutil.ParseQuery(c, shared.NotesRT, shared.NoteQuery)
if !ok { return }
var req Request
if !handlerutil.BindResource(c, handlerutil.ResourceSpec{Type: shared.NotesRT}, &req) { return }
note, err := h.service.Create(c, req, c.GetInt64(internalsharedconstant.UserIDKey))
if handlerutil.WriteError(c, err, "Create Note Handler") { return }
c.Header("Location", path.Join(c.Request.URL.Path, note.ID))
handlerutil.WriteData(c, http.StatusCreated, shared.NotesRT, note, q, nil)
```

Request: `Title string \`json:"title" binding:"required,notblank,max=255" label:"Title"\``.

## 3. Lock before you write: a transaction over two queriers

Used by: `note/update`, `note/delete`. Also (token rows, not an owned resource) `auth/refresh`.

Every update / delete / state transition of an existing row starts with the `FOR UPDATE` lock **inside** the
tx, then writes, then (update) reads back in the same tx. `store.go`:

```go
// Querier is what the transaction runs: the slice's queries plus the domain-shared ones, on one *sql.Tx.
type Querier interface {
	sqlc.Querier
	notesharedsqlc.Querier
}

type StoreI interface {
	RunInTx(ctx context.Context, fn func(q Querier) error) error
}

type Store struct {
	db      *sql.DB
	queries *sqlc.Queries
	shared  *notesharedsqlc.Queries
}

func NewStore(db *sql.DB) StoreI {
	return &Store{db: db, queries: sqlc.New(db), shared: notesharedsqlc.New(db)}
}

// The shared one is embedded under a local alias so the embedded field names don't clash.
type sharedQuerier = notesharedsqlc.Queries

type txQueries struct {
	*sqlc.Queries
	*sharedQuerier
}

func (s *Store) RunInTx(ctx context.Context, fn func(q Querier) error) error {
	return tx.Exec(ctx, s.db, func(t *sql.Tx) Querier {
		return txQueries{Queries: s.queries.WithTx(t), sharedQuerier: s.shared.WithTx(t)}
	}, fn)
}
```

`mockgen -source=store.go` → `MockStoreI` + `MockQuerier`. Service:

```go
err := s.store.RunInTx(ctx, func(q Querier) error {
	if err := shared.LockOwnedNote(ctx, q, id, userID, feature); err != nil { // lock first
		return err
	}
	// ... q.UpdateNote / q.SoftDeleteNote → on error apperror.CodeInternal "We couldn't save/delete ..."
	note, err := shared.GetOwnedNote(ctx, q, id, userID, feature) // read back (update only)
	if err != nil { return err }
	out = shared.NewNoteResource(shared.FieldsFromRow(note))
	return nil
})
```

Soft delete (204, row kept):

```sql
-- name: SoftDeleteNote :exec
UPDATE notes SET deleted_at = now(), deleted_by = $2 WHERE id = $1;
```

called with `sqlc.SoftDeleteNoteParams{ID: id, DeletedBy: utility.NullInt64(userID)}`.

## 4. PATCH is partial: pointer fields + `COALESCE(narg)`

Used by: `note/update`. Variant without SQL: `admin/user` (nothing to change → read back).

```go
type Request struct {
	Title *string `json:"title" binding:"omitempty,notblank,max=255" label:"Title"`
	Body  *string `json:"body" binding:"omitempty,max=10000" label:"Content"`
}
```

```sql
-- name: UpdateNote :exec
-- A NULL title / body keeps the current value. Run after LockNoteForUpdate, in the same tx.
UPDATE notes
SET title = COALESCE(sqlc.narg('title')::varchar, title),
    body  = COALESCE(sqlc.narg('body')::text, body)
WHERE id = sqlc.arg('id');
```

```go
params := sqlc.UpdateNoteParams{ID: id}
if req.Title != nil {
	params.Title = utility.NullString(strings.TrimSpace(*req.Title)) // "" → NULL (kept); notblank forbids it anyway
}
if req.Body != nil {
	params.Body = sql.NullString{String: *req.Body, Valid: true} // an empty body is a real value
}
```

`utility.NullString("")` is NULL, so use it only where "" can't be a wanted value. A column that must be
settable to NULL needs an explicit flag (`clear_x`) — `COALESCE` can't express it.

Handler: the id is required and must equal the URL's (`ResourceSpec{Type, ID: strconv.FormatInt(id, 10)}`),
`handlerutil.ParamID(c, "id", "note")` first (a non-numeric / ≤ 0 id is a 404), 200 with the resource.

## 5. List: filter, one-field sort, pagination

Used by: `note/list`. Also `admin/user` (pagination).

`dto.go`:

```go
var querySpec = jsonapi.QuerySpec{
	Include: &shared.NoteIncludes,
	Fields:  shared.NoteFieldsets,
	Sort:    &jsonapi.SortConfig{Allowed: []string{"created_at", "updated_at", "title"}, Default: "-created_at"},
	Page:    true,
	Filters: []string{"q"},
}

type Request struct {
	Q    string // filter[q]
	Sort string // one querySpec sort value, e.g. "-created_at"
	pagination.Request
}

func requestFromQuery(q jsonapi.Query) Request {
	return Request{Q: q.Filter["q"], Sort: q.Sort, Request: q.Page}
}
```

SQL — optional filter via `narg IS NULL OR`, sort via one `CASE` per allowed value, `id` as tie-breaker in the
same direction, and a count with the **same** filters:

```sql
-- name: ListNotes :many
SELECT n.id, n.title, n.body, n.created_by, n.created_at, n.updated_at,
       u.name AS creator_name, u.email AS creator_email
FROM notes n JOIN users u ON u.id = n.created_by
WHERE n.created_by = sqlc.arg('user_id')
  AND n.deleted_at IS NULL
  AND (sqlc.narg('q')::varchar IS NULL
    OR n.title ILIKE '%' || sqlc.narg('q')::varchar || '%'
    OR n.body ILIKE '%' || sqlc.narg('q')::varchar || '%')
ORDER BY CASE WHEN sqlc.arg('sort')::text = 'title' THEN n.title END,
         CASE WHEN sqlc.arg('sort')::text = '-title' THEN n.title END DESC,
         CASE WHEN sqlc.arg('sort')::text = 'created_at' THEN n.created_at END,
         CASE WHEN sqlc.arg('sort')::text = '-created_at' THEN n.created_at END DESC,
         CASE WHEN sqlc.arg('sort')::text = 'updated_at' THEN n.updated_at END,
         CASE WHEN sqlc.arg('sort')::text = '-updated_at' THEN n.updated_at END DESC,
         CASE WHEN left(sqlc.arg('sort')::text, 1) = '-' THEN n.id END DESC,
         n.id
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: CountNotes :one
-- Same filters as ListNotes.
SELECT count(*) FROM notes n WHERE n.created_by = sqlc.arg('user_id') AND n.deleted_at IS NULL AND (...same q...);
```

Service:

```go
page, pageSize, limit, offset := req.Resolve()
q := utility.NullString(strings.TrimSpace(req.Q))
rows, err := s.querier.ListNotes(ctx, sqlc.ListNotesParams{UserID: userID, Q: q, Sort: req.Sort, Limit: limit, Offset: offset})
// ... CountNotes with the same filters; each error → CodeInternal with one shared loadErrorMessage
notes := make([]shared.NoteResource, 0, len(rows)) // never nil: an empty page is "data": []
// ... map each row onto shared.NoteFields → NewNoteResource
return notes, pagination.NewMeta(page, pageSize, total), nil
```

Handler: `ParseQuery(c, shared.NotesRT, querySpec)` → `service.List(c, requestFromQuery(q), userID)` →
`WriteData(c, 200, shared.NotesRT, notes, q, meta)` (the `pagination.Meta` adds the page links).

## 6. Detail: a slice with no sqlc of its own

Used by: `note/detail`.

Everything it reads is domain-shared, so it has no `query/` / `sqlc/` and no `sqlc.yaml` block:
`NewService(noteSharedQuerier)` → `shared.GetOwnedNote` → `NewNoteResource`. Handler: `ParamID` →
`ParseQuery(..., shared.NoteQuery)` → service → `WriteData(200)`.

## 7. Tests for the patterns above

Used by: every `note/*` slice.

- **Service unit test over a transaction** — `RunInTx` runs `fn` with the mock querier; `gomock.InOrder`
  asserts lock → write → read back; a failed ownership check asserts nothing was written (no further
  `EXPECT`):

```go
store := NewMockStoreI(ctrl)
store.EXPECT().RunInTx(gomock.Any(), gomock.Any()).DoAndReturn(
	func(ctx context.Context, fn func(Querier) error) error { return fn(q) })

gomock.InOrder(
	q.EXPECT().LockNoteForUpdate(gomock.Any(), noteID).Return(owned, nil),
	q.EXPECT().UpdateNote(gomock.Any(), sqlc.UpdateNoteParams{ID: noteID, Title: sql.NullString{String: "New", Valid: true}}).Return(nil),
	q.EXPECT().GetNoteByID(gomock.Any(), noteID).Return(row, nil),
)
```

  Cases to always have: own resource (happy path, exact params incl. trimming), partial update keeps what
  wasn't sent, someone else's → 404 and nothing written, missing → 404, write error → 500.
- **Ownership helper unit test** (`shared/access_unit_test.go`): own / someone else's → 404 / `ErrNoRows` →
  404 / other error → 500, for both `GetOwned*` and `LockOwned*`.
- **Handler unit test**: inject the caller with a middleware, register through the slice's `Route`:

```go
r := gin.New()
r.Use(func(c *gin.Context) { c.Set(internalsharedconstant.UserIDKey, int64(7)); c.Next() })
Route(r.Group("/api"), NewHandler(svc))
```

  Cases: happy path, partial body, empty attributes (no-op), bad id → 404, body id ≠ URL id → 409 with
  `"source":{"pointer":"/data/id"}`, blank → 422 with the `label` text and pointer, read-only attribute → 403,
  service AppError passes through, plain error → 500. `require.NoError(t, validation.RegisterGinValidators())`
  first.
- **Service integration test**: each case has an `arrange(t, db, me, other) int64` that seeds; assert both
  the returned value **and** the stored row (`testhelper.GetX`) — e.g. "someone else's is a 404 *and
  untouched*", "deleted is a 404 and untouched":

```go
defer testhelper.Truncate(t, testDB)
me := testhelper.SeedUser(t, testDB, testhelper.SeedUserParams{})
other := testhelper.SeedUser(t, testDB, testhelper.SeedUserParams{})
id := tc.arrange(t, testDB, me, other)
got, err := NewService(NewStore(testDB)).Update(context.Background(), id, tc.req, me)
stored := testhelper.GetNote(t, testDB, id)
```

- **testhelper** for a new table: `SeedXParams` with defaults for "" / zero and a `Deleted bool` that
  soft-deletes in the same insert; `GetX` reads the row **deleted or not** (all columns incl. `deleted_at` /
  `deleted_by`); add the table to `Truncate`:

```go
err := db.QueryRow(`
	INSERT INTO notes (title, body, created_by, deleted_at, deleted_by)
	VALUES ($1, $2, $3,
	        CASE WHEN $4::boolean THEN now() END,
	        CASE WHEN $4::boolean THEN $3::bigint END)
	RETURNING id
`, p.Title, p.Body, p.CreatedBy, p.Deleted).Scan(&id)
```

## 8. sqlc block per slice

Used by: every slice with its own `query/`.

```yaml
  - engine: "postgresql"
    schema: "internal/platform/db/migration"
    queries: "internal/features/note/shared/query"
    gen:
      go:
        package: "sqlc"
        out: "internal/features/note/shared/sqlc"
        emit_json_tags: true
        emit_prepared_queries: false
        emit_interface: true
        emit_exact_table_names: false
        emit_empty_slices: true
        omit_unused_structs: true
```

Then `make sqlc` and `make mock SOURCE=<slice>/sqlc/querier.go DESTINATION=<slice>/sqlc/mock_querier.go PKG_NAME=sqlc`.
