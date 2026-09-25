-- Dropping this table loses every generated ROI review report: the stored
-- parameters, the copies of the records each report read and the results
-- people have already seen. 会丢失已登记的真实经营数据.
DROP TABLE IF EXISTS content_roi_report_version;
