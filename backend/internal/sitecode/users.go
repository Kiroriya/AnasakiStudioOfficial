package sitecode

// db.users (CMS 2.2.2, scope "users"): read-only public profiles of the site's users — author pages, team lists,
// bylines with an avatar. Never the email, status, login times or capabilities; suspended users are left out.

import (
	"strconv"
	"strings"

	"github.com/anasaki/studio/backend/internal/model"
	"github.com/dop251/goja"
)

// UserStore is what db.users reads (the CMS store).
type UserStore interface {
	ListUsers(f model.UserFilter) ([]model.User, int, error)
	UserByID(id int64) (*model.User, error)
}

const maxUsersPerPage = 100

func (v *vm) userStore(what string) UserStore {
	us, ok := v.h.posts.(UserStore)
	if !ok || us == nil {
		v.throw("%s ไม่พร้อมใช้งานในสภาพแวดล้อมนี้", what)
	}
	return us
}

// publicProfile is the part of a user any code may show on the site.
func publicProfile(u *model.User) map[string]any {
	return map[string]any{"id": u.ID, "name": u.Name, "firstName": u.FirstName, "lastName": u.LastName, "bio": u.Bio,
		"website": u.Website, "avatar": u.Avatar, "role": u.Role, "postCount": u.PostCount}
}

// db.users.list({ role?, search?, page?, perPage? }) → { items, total, page, perPage }
func (v *vm) usersList(c goja.FunctionCall) goja.Value {
	const what = "db.users.list"
	v.handlerOnly("db")
	v.need("users", what)
	o := optObject(v.rt, c.Argument(0))
	page, perPage := 1, 20
	if s := optString(o, "page"); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n < 1 {
			v.throw("%s: page ต้องเป็นเลข 1 ขึ้นไป", what)
		}
		page = n
	}
	if s := optString(o, "perPage"); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n < 1 || n > maxUsersPerPage {
			v.throw("%s: perPage ต้องอยู่ระหว่าง 1 ถึง %d", what, maxUsersPerPage)
		}
		perPage = n
	}
	f := model.UserFilter{Search: strings.TrimSpace(optString(o, "search")), Role: strings.TrimSpace(optString(o, "role")),
		Status: model.UserActive, Limit: perPage, Offset: (page - 1) * perPage}
	list, total, err := v.userStore(what).ListUsers(f)
	if err != nil {
		v.fail("%s: %v", what, err)
	}
	items := make([]any, 0, len(list))
	for i := range list {
		items = append(items, publicProfile(&list[i]))
	}
	return v.toJS(map[string]any{"items": items, "total": total, "page": page, "perPage": perPage})
}

// db.users.get(id) → profile | null (also null for a suspended user)
func (v *vm) usersGet(c goja.FunctionCall) goja.Value {
	const what = "db.users.get"
	v.handlerOnly("db")
	v.need("users", what)
	id := anyID(c.Argument(0).Export())
	if id < 1 {
		v.throw("%s: ต้องส่ง id ของผู้ใช้", what)
	}
	u, err := v.userStore(what).UserByID(id)
	if err != nil || u == nil || u.Status != model.UserActive {
		return goja.Null()
	}
	return v.toJS(publicProfile(u))
}
