package domain

import (
	"errors"
	"testing"
)

func TestCanViewRejectsHighRiskForNonDevelopers(t *testing.T) {
	// C-20：高风险类别与「开发者可见」同等处理。即便可见性为 public，
	// 对普通用户与游客一律不可见。
	access := Access{Level: VisibilityPublic, HighRisk: true, UploaderID: 42}

	if Guest().CanView(access) {
		t.Error("游客不应看到高风险类别的公开提示词")
	}
	ordinary := Viewer{UserID: 7, Role: RoleUser, Logged: true}
	if ordinary.CanView(access) {
		t.Error("普通用户不应看到高风险类别的公开提示词，即使该提示词属于自己以外的用户")
	}
	// 上传者本人同样受高风险约束：这条内容一旦进入高风险类别，
	// 就按 C-20 与开发者可见同等处理，不再对普通身份开放。
	if ordinary.CanView(Access{Level: VisibilityPublic, HighRisk: true, UploaderID: 7}) {
		t.Error("高风险类别对普通身份一律不可见，含上传者本人")
	}

	developer := Viewer{UserID: 7, Role: RoleDeveloper, Logged: true}
	if !developer.CanView(access) {
		t.Error("开发者应能看到高风险类别的提示词")
	}
	if !ViewerFor(User{ID: 1, Role: RoleAdmin, Status: StatusNormal}).CanView(access) {
		t.Error("管理员隐含开发者权限（C-17），应能看到高风险类别的提示词")
	}
}

func TestCanViewRespectsVisibilityLevels(t *testing.T) {
	owner := Viewer{UserID: 7, Role: RoleUser, Logged: true}
	other := Viewer{UserID: 9, Role: RoleUser, Logged: true}
	developer := Viewer{UserID: 9, Role: RoleDeveloper, Logged: true}
	admin := ViewerFor(User{ID: 1, Role: RoleAdmin, Status: StatusNormal})

	cases := []struct {
		name   string
		access Access
		want   map[string]bool
	}{
		{
			name:   "公开提示词对所有人可见",
			access: Access{Level: VisibilityPublic, UploaderID: 7},
			want:   map[string]bool{"guest": true, "owner": true, "other": true, "developer": true, "admin": true},
		},
		{
			name:   "私有提示词仅所有者与管理员可见",
			access: Access{Level: VisibilityPrivate, UploaderID: 7},
			want:   map[string]bool{"guest": false, "owner": true, "other": false, "developer": false, "admin": true},
		},
		{
			name:   "开发者可见提示词仅开发者与管理员可见",
			access: Access{Level: VisibilityDeveloper, UploaderID: 7},
			want:   map[string]bool{"guest": false, "owner": false, "other": false, "developer": true, "admin": true},
		},
	}

	viewers := map[string]Viewer{
		"guest":     Guest(),
		"owner":     owner,
		"other":     other,
		"developer": developer,
		"admin":     admin,
	}
	for _, tc := range cases {
		for who, want := range tc.want {
			if got := viewers[who].CanView(tc.access); got != want {
				t.Errorf("%s：%s 的判定 = %v，期望 %v", tc.name, who, got, want)
			}
		}
	}
}

func TestBannedViewerLosesAllAuthorization(t *testing.T) {
	// C-24：封禁账户会话立即失效，可见性判定退化为游客。
	v := ViewerFor(User{ID: 3, Role: RoleAdmin, Status: StatusBanned})
	if v.Logged {
		t.Error("封禁账户不应持有已登录身份")
	}
	if v.CanWrite() || v.CanUploadDeveloper() || v.CanUseHighRiskCategory() || v.IsAdmin() || v.IsDeveloper() {
		t.Error("封禁账户不应保有任何授权")
	}
	if v.CanView(Access{Level: VisibilityPublic, UploaderID: 3}) != true {
		t.Error("封禁账户对公开内容仍可浏览")
	}
	if v.CanView(Access{Level: VisibilityPrivate, UploaderID: 3}) {
		t.Error("封禁账户不应看到自己的私有内容")
	}
}

func TestAuthorizeCategoryBlocksOrdinaryUsersOnBothRules(t *testing.T) {
	ordinary := Viewer{UserID: 7, Role: RoleUser, Logged: true}
	developer := Viewer{UserID: 7, Role: RoleDeveloper, Logged: true}

	// 创建与更新两条路径共用 AuthorizeCategory，此处覆盖它对两项规则的全部取值组合。
	cases := []struct {
		highRisk bool
		visible  Visibility
		wantErr  error
	}{
		{highRisk: false, visible: VisibilityPublic},
		{highRisk: false, visible: VisibilityPrivate},
		{highRisk: false, visible: VisibilityDeveloper, wantErr: ErrDeveloperVisibilityForbidden},
		{highRisk: true, visible: VisibilityPublic, wantErr: ErrHighRiskCategoryForbidden},
		{highRisk: true, visible: VisibilityPrivate, wantErr: ErrHighRiskCategoryForbidden},
		{highRisk: true, visible: VisibilityDeveloper, wantErr: ErrHighRiskCategoryForbidden},
	}
	for _, tc := range cases {
		got := ordinary.AuthorizeCategory(tc.highRisk, tc.visible)
		if !errors.Is(got, tc.wantErr) {
			t.Errorf("普通用户 high_risk=%v visibility=%s 的判定 = %v，期望 %v",
				tc.highRisk, tc.visible, got, tc.wantErr)
		}
		if err := developer.AuthorizeCategory(tc.highRisk, tc.visible); err != nil {
			t.Errorf("开发者 high_risk=%v visibility=%s 应当通过，实际 %v", tc.highRisk, tc.visible, err)
		}
	}
}

func TestListScopeExcludesHighRiskForNonDevelopers(t *testing.T) {
	guestScope, guestArgs := Guest().ListScope()
	developerScope, developerArgs := ViewerFor(User{Role: RoleDeveloper, Status: StatusNormal}).ListScope()

	if guestArgs != nil || developerArgs != nil {
		t.Error("ListScope 的谓词不携带参数")
	}
	// 两条谓词都只收录 public 档位，这是公开列表的定义（需求 §8.2）。
	if !containsFragment(guestScope, "p.visibility = 'public'") || !containsFragment(developerScope, "p.visibility = 'public'") {
		t.Errorf("公开列表谓词应收录 public 档位，实际 guest=%q developer=%q", guestScope, developerScope)
	}
	// 非开发者排除高风险；开发者不加该过滤，否则其看不到自己有权访问的内容。
	if !containsFragment(guestScope, "is_high_risk") {
		t.Errorf("游客谓词应排除高风险类别，实际 %q", guestScope)
	}
	if containsFragment(developerScope, "is_high_risk") {
		t.Errorf("开发者谓词不应排除高风险类别，实际 %q", developerScope)
	}
}

func containsFragment(s, fragment string) bool {
	for i := 0; i+len(fragment) <= len(s); i++ {
		if s[i:i+len(fragment)] == fragment {
			return true
		}
	}
	return false
}

func TestCanManageGrantsOwnerAndAdmin(t *testing.T) {
	access := Access{Level: VisibilityPublic, UploaderID: 7}
	owner := Viewer{UserID: 7, Role: RoleUser, Logged: true}
	other := Viewer{UserID: 9, Role: RoleUser, Logged: true}
	admin := ViewerFor(User{ID: 1, Role: RoleAdmin, Status: StatusNormal})

	if !owner.CanManage(access) {
		t.Error("所有者应可管理自己的提示词")
	}
	if other.CanManage(access) {
		t.Error("无关用户不应可管理他人提示词")
	}
	if !admin.CanManage(access) {
		t.Error("管理员应可管理任意提示词（C-18）")
	}
}
