-- Dropping this table loses every account profile change proposal and
-- whether it was confirmed or dismissed; confirmed changes stay in
-- ip-profile's own revisions. 会丢失账号配置修改提议及其确认记录.
DROP TABLE IF EXISTS content_opdiag_profile_proposal_revision;
