export {};

declare global {
  /** Minimal shapes for the legacy soybean request layer, which still
   *  references these globals with optional chaining. No live code assigns
   *  them now that the console uses TDesign plugins throughout. */
  interface LegacyDialogOptions {
    title?: string;
    content?: string;
    positiveText?: string;
    maskClosable?: boolean;
    closeOnEsc?: boolean;
    onPositiveClick?: () => void;
    onClose?: () => void;
  }
  interface LegacyMessageOptions {
    onLeave?: () => void;
    duration?: number;
  }
  interface LegacyMessageApi {
    error(message: string, options?: LegacyMessageOptions): void;
    warning(message: string, options?: LegacyMessageOptions): void;
    success(message: string, options?: LegacyMessageOptions): void;
    info(message: string, options?: LegacyMessageOptions): void;
  }

  export interface Window {
    /** NProgress instance */
    NProgress?: import('nprogress').NProgress;
    $dialog?: { error(options: LegacyDialogOptions): void };
    $message?: LegacyMessageApi;
  }

  /** Build time of the project */
  export const BUILD_TIME: string;
}
