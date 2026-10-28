-- 문서 모양(스펙 7.3절)의 기본 항목. options_json '{}'는 앱 기본값(export.DefaultOptions)을 쓴다는 뜻이고, 첫 항목이라 지울 수 없다.
INSERT INTO export_presets(name, format, options_json) VALUES ('기본', 'pdf', '{}');
