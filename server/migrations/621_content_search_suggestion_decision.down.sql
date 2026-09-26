-- Dropping this table loses every decision taken on a search optimization
-- suggestion: who abandoned or adopted which one, and when. 会丢失全部建议决定.
DROP TABLE IF EXISTS content_search_suggestion_decision;
