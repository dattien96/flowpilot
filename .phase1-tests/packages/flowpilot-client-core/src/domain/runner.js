"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
exports.LoadRunnerHealthUseCase = void 0;
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
