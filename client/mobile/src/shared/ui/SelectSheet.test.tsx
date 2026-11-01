import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { SelectSheet } from "./SelectSheet";

afterEach(cleanup);

it("filters options ignoring accents when searchable", () => {
  const onChange = vi.fn();
  render(
    <SelectSheet
      label="Tỉnh/thành"
      searchable
      value=""
      onChange={onChange}
      options={[
        { value: "DN", label: "Thành phố Đà Nẵng" },
        { value: "HN", label: "Thành phố Hà Nội" },
      ]}
    />,
  );
  fireEvent.click(screen.getByRole("button", { name: /^Tỉnh\/thành:/ }));
  fireEvent.change(screen.getByLabelText("Tìm Tỉnh/thành"), {
    target: { value: "da nang" },
  });

  expect(screen.queryByRole("button", { name: "Thành phố Hà Nội" })).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "Thành phố Đà Nẵng" }));
  expect(onChange).toHaveBeenCalledWith("DN");
});
