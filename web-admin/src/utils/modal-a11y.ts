/**
 * Accessibility patch for tdesign-vue-next 1.20 dialogs/drawers.
 *
 * The library renders `.t-dialog` / `.t-drawer` containers without
 * `role="dialog"` or `aria-modal="true"`, so assistive technology treats the
 * modal content as part of the page. This observer adds the missing semantics
 * whenever a modal mounts, without forking the component library.
 */
const DIALOG_SELECTOR = '.t-dialog';
const DRAWER_SELECTOR = '.t-drawer';
const LABEL_FALLBACK = 'dialog';

function labelOf(container: Element): string {
  const header = container.querySelector('.t-dialog__header-content, .t-drawer__header');
  const text = (header?.textContent || '').trim();
  if (!text) return LABEL_FALLBACK;
  if (!header!.id) header!.id = `a11y-label-${Math.random().toString(36).slice(2, 9)}`;
  return header!.id;
}

function patchModal(container: Element) {
  if (container.getAttribute('role') !== 'dialog') {
    container.setAttribute('role', 'dialog');
    container.setAttribute('aria-modal', 'true');
    const id = labelOf(container);
    if (id !== LABEL_FALLBACK) container.setAttribute('aria-labelledby', id);
  }
}

function patchAll(root: ParentNode = document) {
  root.querySelectorAll(DIALOG_SELECTOR).forEach(patchModal);
  root.querySelectorAll(DRAWER_SELECTOR).forEach(patchModal);
}

/** Start observing modal mounts. Safe to call once at app boot. */
export function setupModalA11y() {
  patchAll();
  const observer = new MutationObserver(mutations => {
    for (const mutation of mutations) {
      // Class changes flip form items in and out of the error state.
      if (mutation.type === 'attributes' && mutation.target instanceof Element) {
        const item = mutation.target.closest('.t-form__item');
        if (item) patchFormItem(item);
        continue;
      }
      for (const node of Array.from(mutation.addedNodes)) {
        if (!(node instanceof HTMLElement)) continue;
        if (node.matches(DIALOG_SELECTOR) || node.matches(DRAWER_SELECTOR)) patchModal(node);
        else patchAll(node);
        patchForms(node);
        patchIconOnly(node);
      }
    }
  });
  observer.observe(document.body, {
    childList: true,
    subtree: true,
    attributes: true,
    attributeFilter: ['class']
  });
  patchForms();
  patchIconOnly();
  return () => observer.disconnect();
}
/* ---------------------------------------------------------------------------
 * Form validation semantics (WCAG 3.3.1 / 4.1.3).
 * tdesign-vue-next renders the error text in `.t-input__extra` but never links
 * it to the control: the input keeps no id, no aria-invalid and no
 * aria-describedby, so a screen-reader user is not told which field failed or
 * why. Patch the relationship whenever a form item enters the error state.
 * ------------------------------------------------------------------------- */
const ERROR_CONTAINER = '.t-form__controls.t-is-error';

let fieldSeq = 0;

function patchFormItem(item: Element) {
  const input = item.querySelector('input, textarea, .t-select__input');
  if (!input) return;

  if (!input.id) {
    fieldSeq += 1;
    input.id = `a11y-field-${fieldSeq}`;
  }

  const errorNode = item.querySelector('.t-input__extra, .t-form__item-error, .t-form__item-help');
  const hasError = Boolean(item.querySelector(ERROR_CONTAINER));

  if (hasError) {
    input.setAttribute('aria-invalid', 'true');
    if (errorNode) {
      if (!errorNode.id) {
        fieldSeq += 1;
        errorNode.id = `a11y-error-${fieldSeq}`;
      }
      input.setAttribute('aria-describedby', errorNode.id);
    }
  } else {
    input.removeAttribute('aria-invalid');
    input.removeAttribute('aria-describedby');
  }

  // Required state is conveyed only by a red asterisk in the label.
  const label = item.querySelector('.t-form__label');
  if (label?.classList.contains('t-form__label--required')) {
    input.setAttribute('aria-required', 'true');
  }
}

function patchForms(root: ParentNode = document) {
  root.querySelectorAll('.t-form__item').forEach(patchFormItem);
}
/* ---------------------------------------------------------------------------
 * Icon-only control names (WCAG 4.1.2).
 * tdesign-vue-next renders TInputNumber steppers as two icon-only buttons
 * (.t-input-number__increase / __decrease) with no accessible name, so a
 * screen reader announces them as just "button". Derive a name from the
 * nearest form label plus the action.
 * ------------------------------------------------------------------------- */
const STEPPER_SELECTOR = '.t-input-number__increase, .t-input-number__decrease';

function labelForStepper(button: Element): string {
  const item = button.closest('.t-form__item');
  const labelEl = item?.querySelector('.t-form__label label, .t-form__label');
  const rawLabel = (labelEl?.textContent || '').trim().replace(/\*$/, '').trim();
  const isIncrease = button.classList.contains('t-input-number__increase');
  const action = isIncrease ? 'increase' : 'decrease';
  if (!rawLabel) return action;
  return rawLabel + ' ' + action;
}

function patchSteppers(root: ParentNode = document) {
  root.querySelectorAll(STEPPER_SELECTOR).forEach(button => {
    if (button.getAttribute('aria-label')) return;
    button.setAttribute('aria-label', labelForStepper(button));
  });
}

/** Patch icon-only controls that the component library leaves unnamed. */
function patchIconOnly(root: ParentNode = document) {
  patchSteppers(root);
}
