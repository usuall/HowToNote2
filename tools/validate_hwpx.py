# HWPX 검증(python-hwpx): 스키마(header.xml, section0.xml)와 패키지 구조(mimetype, manifest, BinData). 사용: python tools/validate_hwpx.py 파일.hwpx
import sys
from hwpx.document import HwpxDocument
from hwpx.tools.package_validator import validate_package

report = HwpxDocument.open(sys.argv[1]).validate()
package = validate_package(sys.argv[1])
if report.issues or not package.ok:
    for issue in list(report.issues) + list(package.issues):
        print(issue)
    sys.exit(1)
print("OK", ", ".join(report.validated_parts), "+ package")
