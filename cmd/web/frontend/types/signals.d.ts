export declare interface Signal<T> {
    (): Readonly<T>;
    update(valueOrFn: T | ((curr: T) => T)): void;
}

export declare type UnlinkFn = (effect: EffectInstance) => void;

export declare interface EffectInstance {
    execute: () => void;
    link: (unlink: UnlinkFn) => void;
}