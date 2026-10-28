# HWPX 기본 템플릿을 만든다. 사용: python tools/make_hwpx_template.py internal/hwpx/template/blank.hwpx
# 1x1 표를 한 번 넣는 이유: header.xml에 실선 테두리(borderFill id 3)를 만들기 위해서다.
# 본문은 Go 작성기가 첫 문단(쪽 설정 secPr)만 쓰고 버리므로 표가 남아도 상관없다.
import sys
from hwpx.document import HwpxDocument

doc = HwpxDocument.new()
doc.add_table(1, 1)
doc.save_to_path(sys.argv[1])
