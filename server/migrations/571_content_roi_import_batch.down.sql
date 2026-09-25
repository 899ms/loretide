-- Dropping this table loses the record of every import: who pasted what, when,
-- and which rows were held back as possible duplicates. The imported records
-- themselves stay in their own tables. 会丢失已登记的真实经营数据的导入留痕.
DROP TABLE IF EXISTS content_roi_import_batch;
