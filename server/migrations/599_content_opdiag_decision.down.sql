-- Dropping this table loses the record of every adopt and reject decision
-- people took on suggestions (D14-V05). 会丢失采纳与拒绝的决定记录.
DROP TABLE IF EXISTS content_opdiag_decision;
