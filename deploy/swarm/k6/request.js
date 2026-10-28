import http from "k6/http";
import { check } from "k6";

export const options = {
  thresholds: {
    http_req_failed: ["rate<0.01"],
    http_req_duration: ["p(95)<1000"],
  },
};

export default function () {
  const response = http.get(__ENV.TARGET_URL || "http://localhost:19100/metrics");
  check(response, {
    "target responds": (value) => value.status >= 200 && value.status < 400,
  });
}
