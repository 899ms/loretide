-- Dropping this table loses how every shared cost was split: operating data a
-- person decided and entered, which exists nowhere else. 会丢失已登记的真实经营数据.
DROP TABLE IF EXISTS content_roi_cost_allocation;
