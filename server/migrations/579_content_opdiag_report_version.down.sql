-- Dropping this table loses every generated operating diagnosis report: the
-- stored parameters, the copies of the inputs each version read and the
-- results people have already seen. 会丢失已生成的诊断报告.
DROP TABLE IF EXISTS content_opdiag_report_version;
