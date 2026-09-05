package domain

import "server/internal/domain/entity"

// ProjectMovieToSearch 从影片详情投影出检索/卡片宽表行 (cover 进表, 列表零回填)。
// service 手动加片与 spider 采集双写共用, 保证两条写路径投影一致。
func ProjectMovieToSearch(m *entity.Movie) *entity.MovieSearch {
	return &entity.MovieSearch{
		Mid: m.Mid, Cid: m.Cid, Pid: m.Pid, Name: m.Name, SubTitle: m.SubTitle,
		CName: m.CName, ClassTag: m.ClassTag, Area: m.Area, Language: m.Language,
		Year: m.Year, Initial: m.Initial, NamePinyin: NameInitials(m.Name),
		State: m.State, Remarks: m.Remarks,
		DbScore: m.DbScore, Hits: m.Hits, Cover: m.Cover,
		ReleaseStamp: m.ReleaseStamp, UpdateStamp: m.UpdateStamp,
	}
}
