package sourceinbox

import "context"

// DuplicatesByHash answers "has this brand collected this exact content
// before" with the ids that already hold it.
//
// It only reads. There is no merge here and no delete: §4 says "重复素材先提示
// 合并关联" - offer the link, let a person decide - and R-011 says "内容相同不
// 删除独立的收藏上下文与批注", because two collections of the same text carry
// two different annotations, times and people, and throwing one away throws
// those away with it.
//
// The comparison is over the exact stored bytes (see ContentHash). Deciding
// that two near-identical texts are "the same" is the judgement being left to
// a person.
func (s *Store) DuplicatesByHash(ctx context.Context, workspaceID, actor, hash string) ([]string, error) {
	if s == nil || s.DB == nil {
		if s != nil {
			s.reportFailure(ctx, workspaceID, actor, "", "duplicates", ErrStorage)
		}
		return nil, ErrStorage
	}
	if workspaceID == "" || actor == "" || hash == "" {
		s.reportFailure(ctx, workspaceID, actor, "", "duplicates", ErrInvalid)
		return nil, ErrInvalid
	}
	rows, err := s.DB.Query(ctx, `SELECT source_id FROM content_source_snapshot
		WHERE workspace_id = $1 AND content_hash = $2
		ORDER BY captured_at, snapshot_id`, workspaceID, hash)
	if err != nil {
		s.reportFailure(ctx, workspaceID, actor, "", "duplicates", err)
		return nil, ErrStorage
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if scanErr := rows.Scan(&id); scanErr != nil {
			s.reportFailure(ctx, workspaceID, actor, "", "duplicates", scanErr)
			return nil, ErrStorage
		}
		ids = append(ids, id)
	}
	if rows.Err() != nil {
		s.reportFailure(ctx, workspaceID, actor, "", "duplicates", rows.Err())
		return nil, ErrStorage
	}
	return ids, nil
}
