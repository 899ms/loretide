-- Dropping this table loses every deal anyone registered: real operating data
-- that exists nowhere else, since the system never fetched it and cannot fetch
-- it again. 会丢失已登记的真实经营数据.
DROP TABLE IF EXISTS content_roi_deal_revision;
