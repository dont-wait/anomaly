import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { Pagination } from "./Pagination";

afterEach(cleanup);

const noop = () => undefined;

it("hides when everything fits on one page", () => {
  render(<Pagination page={1} pageCount={1} onPrevious={noop} onNext={noop} />);
  expect(screen.queryByRole("navigation")).toBeNull();
});

it("disables the ends and allows next while the server has more", () => {
  const onNext = vi.fn();
  render(
    <Pagination
      page={1}
      pageCount={1}
      hasMore
      onPrevious={noop}
      onNext={onNext}
    />,
  );
  expect(
    screen.getByRole<HTMLButtonElement>("button", { name: "Trang trước" })
      .disabled,
  ).toBe(true);
  fireEvent.click(screen.getByRole("button", { name: "Trang sau" }));
  expect(onNext).toHaveBeenCalledTimes(1);
});
