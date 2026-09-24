-- Dropping this table loses every attribution judgement anyone recorded: the
-- operator's own reading of where each deal came from, which exists nowhere
-- else. 会丢失已登记的真实经营数据.
DROP TABLE IF EXISTS content_roi_attribution_revision;
