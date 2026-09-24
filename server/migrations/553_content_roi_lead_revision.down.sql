-- Dropping this table loses every lead anyone registered: real operating data
-- that exists nowhere else, since the system never fetched it and cannot fetch
-- it again. 会丢失已登记的真实经营数据.
DROP TABLE IF EXISTS content_roi_lead_revision;
