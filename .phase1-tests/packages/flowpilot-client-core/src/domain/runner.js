"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
exports.RunCompatDeepCheckUseCase = exports.RunCompatCheckUseCase = exports.SaveCompatConfigUseCase = exports.LoadCompatConfigUseCase = exports.LoadCompatInfoUseCase = exports.LoadRunnerHealthUseCase = void 0;
class LoadRunnerHealthUseCase {
    runnerRepository;
    constructor(runnerRepository) {
        this.runnerRepository = runnerRepository;
    }
    execute() {
        return this.runnerRepository.loadHealth();
    }
}
exports.LoadRunnerHealthUseCase = LoadRunnerHealthUseCase;
class LoadCompatInfoUseCase {
    runnerRepository;
    constructor(runnerRepository) {
        this.runnerRepository = runnerRepository;
    }
    execute() {
        return this.runnerRepository.loadCompatInfo();
    }
}
exports.LoadCompatInfoUseCase = LoadCompatInfoUseCase;
class LoadCompatConfigUseCase {
    runnerRepository;
    constructor(runnerRepository) {
        this.runnerRepository = runnerRepository;
    }
    execute() {
        return this.runnerRepository.loadCompatConfig();
    }
}
exports.LoadCompatConfigUseCase = LoadCompatConfigUseCase;
class SaveCompatConfigUseCase {
    runnerRepository;
    constructor(runnerRepository) {
        this.runnerRepository = runnerRepository;
    }
    execute(input) {
        return this.runnerRepository.saveCompatConfig(input);
    }
}
exports.SaveCompatConfigUseCase = SaveCompatConfigUseCase;
class RunCompatCheckUseCase {
    runnerRepository;
    constructor(runnerRepository) {
        this.runnerRepository = runnerRepository;
    }
    execute() {
        return this.runnerRepository.runCompatCheck();
    }
}
exports.RunCompatCheckUseCase = RunCompatCheckUseCase;
class RunCompatDeepCheckUseCase {
    runnerRepository;
    constructor(runnerRepository) {
        this.runnerRepository = runnerRepository;
    }
    execute() {
        return this.runnerRepository.runCompatDeepCheck();
    }
}
exports.RunCompatDeepCheckUseCase = RunCompatDeepCheckUseCase;
