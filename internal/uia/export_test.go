// 테스트 전용 노출. uia2Timeouts는 M1-carry-1 되읽기 확인용일 뿐 운영 코드의 공개 API가 아니다.
package uia

var UIA2TimeoutsForTest = (*Service).uia2Timeouts
