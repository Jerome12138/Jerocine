package spider

import (
	"context"
	"testing"

	"server/internal/domain/entity"
	"server/internal/domain/repository"
)

// fakeCatRepo 只覆写 All: backfillPid 的分类父映射来源。
type fakeCatRepo struct {
	repository.CategoryRepository
	cats []entity.Category
}

func (f *fakeCatRepo) All(context.Context) ([]entity.Category, error) { return f.cats, nil }

// TestBackfillPid_ChildResolved / 直挂保持 pid=0:
// XML 源恒写 pid=0 —— cid 是子分类的影片回填父顶级 id(与 maccms JSON 口径一致);
// cid 是顶级(直挂)的保持 pid=0, 由读取侧 (pid=X OR cid=X) 口径兜底, 且保证 pid≠cid
// (后台计数 pids+cids 相加不重复)。
func TestBackfillPid(t *testing.T) {
	e := &Engine{category: &fakeCatRepo{cats: []entity.Category{
		{Id: 1, Pid: 0}, // 顶级: AI漫剧
		{Id: 2, Pid: 1}, // 子级
		{Id: 3, Pid: 0},
	}}}
	movies := []entity.Movie{
		{Mid: 1, Cid: 2, Pid: 0}, // 子分类影片 → pid 回填 1
		{Mid: 2, Cid: 1, Pid: 0}, // 顶级直挂 → 保持 0
		{Mid: 3, Cid: 2, Pid: 9}, // 已有 pid 不动
		{Mid: 4, Cid: 0, Pid: 0}, // 无分类, 不动
		{Mid: 5, Cid: 3, Pid: 0}, // 顶级直挂 → 保持 0
	}
	e.backfillPid(context.Background(), movies)
	want := []int64{1, 0, 9, 0, 0}
	for i, w := range want {
		if movies[i].Pid != w {
			t.Fatalf("movies[%d].Pid = %d, want %d", i, movies[i].Pid, w)
		}
	}
}

// TestBackfillPid_CategoryErrorBestEffort 分类查询失败时按原样落库, 不阻塞采集。
func TestBackfillPid_CategoryErrorBestEffort(t *testing.T) {
	e := &Engine{category: &fakeCatErr{}}
	movies := []entity.Movie{{Mid: 1, Cid: 2, Pid: 0}}
	e.backfillPid(context.Background(), movies)
	if movies[0].Pid != 0 {
		t.Fatalf("查询失败应原样保留 pid=0, got %d", movies[0].Pid)
	}
}

type fakeCatErr struct{ repository.CategoryRepository }

func (f *fakeCatErr) All(context.Context) ([]entity.Category, error) {
	return nil, context.DeadlineExceeded
}
