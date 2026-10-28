-- 편집기에서 지운 스텝은 가이드를 닫을 때까지 되살릴 수 있게 표시만 한다(스펙 6.3절). 가이드를 닫을 때와 앱을 열 때 지운다.
ALTER TABLE steps ADD COLUMN deleted INTEGER NOT NULL DEFAULT 0;
