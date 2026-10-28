# 내보낸 파일(PDF·HWPX·DOCX·HTML)에 로컬 경로, AppData, 인쇄용 가상 호스트, 사용자 이름이 들어갔는지 찾는다.
# 쓰는 법: python tools/leakscan.py 파일...  → 찾은 것마다 '발견:' 한 줄, 끝에 '검사 끝'.
import os, re, sys, zipfile, zlib

bad = [rb"c:\users", b"c:/users", b"appdata", b"howtonote.example", os.environ["USERNAME"].lower().encode()]
for p in sys.argv[1:]:
    data = open(p, "rb").read()
    parts = [data]
    if zipfile.is_zipfile(p):
        z = zipfile.ZipFile(p)
        parts = [z.read(n) for n in z.namelist() if not n.endswith(".png")]
    if p.lower().endswith(".pdf"):
        for m in re.finditer(rb"stream\r?\n(.*?)endstream", data, re.S):
            try:
                parts.append(zlib.decompress(m.group(1)))
            except zlib.error:
                pass
    for d in parts:
        # html/template은 data URI의 '+'를 &#43;로 쓴다. 풀지 않으면 base64가 거기서 끊겨 그림 데이터가 글자로 검사된다
        d = d.replace(b"&#43;", b"+")
        low = re.sub(rb"base64,[A-Za-z0-9+/=]+", b"", d).lower()
        for b in bad:
            if b in low:
                print("발견:", p, b)
print("검사 끝")
