export interface Feature {
  id: string;
  projectId: string;
  title: string;
  businessGoal: string;
  userProblem: string;
  expectedFlow: string;
  acceptanceCriteria: string;
  priority: "low" | "medium" | "high";
  status: "draft" | "active" | "completed";
  ownerId: string;
  createdAt: string;
  updatedAt: string;
}
